package database

import (
	"fmt"

	crd "cloud.redhat.com/clowder/v2/apis/cloud.redhat.com/v1alpha1"
	"cloud.redhat.com/clowder/v2/controllers/cloud.redhat.com/config"
	"cloud.redhat.com/clowder/v2/controllers/cloud.redhat.com/errors"
	"cloud.redhat.com/clowder/v2/controllers/cloud.redhat.com/providers"
	"cloud.redhat.com/clowder/v2/controllers/cloud.redhat.com/utils"

	p "cloud.redhat.com/clowder/v2/controllers/cloud.redhat.com/providers"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type localDbProvider struct {
	p.Provider
	Config config.DatabaseConfig
}

func NewLocalDBProvider(p *p.Provider) (providers.ClowderProvider, error) {
	return &localDbProvider{Provider: *p}, nil
}

// CreateDatabase ensures a database is created for the given app.  The
// namespaced name passed in must be the actual name of the db resources
func (db *localDbProvider) Provide(app *crd.ClowdApp, c *config.AppConfig) error {
	if app.Spec.Database.Name == "" {
		return nil
	}

	nn := types.NamespacedName{
		Name:      fmt.Sprintf("%v-db", app.Name),
		Namespace: app.Namespace,
	}

	dd := apps.Deployment{}
	exists, err := utils.UpdateOrErr(db.Client.Get(db.Ctx, nn, &dd))

	if err != nil {
		return err
	}

	dbCfg := config.DatabaseConfig{}
	dataInit := func() map[string]string {
		return map[string]string{
			"hostname": fmt.Sprintf("%v.%v.svc", nn.Name, nn.Namespace),
			"port":     "5432",
			"username": utils.RandString(16),
			"password": utils.RandString(16),
			"pgPass":   utils.RandString(16),
			"name":     app.Spec.Database.Name,
		}
	}

	secMap, err := config.MakeOrGetSecret(db.Ctx, app, db.Client, nn, dataInit)
	if err != nil {
		return errors.Wrap("Couldn't set/get secret", err)
	}

	dbCfg.Populate(secMap)
	dbCfg.AdminUsername = "postgres"

	db.Config = dbCfg
	var image string
	if db.Env.Spec.Providers.Database.Image != "" {
		image = db.Env.Spec.Providers.Database.Image
	} else {
		image = ""
		for _, img := range db.Env.Spec.Providers.Database.ImageList {
			if *app.Spec.Database.Version == img.Version {
				image = img.Image
				break
			}
		}

		if image == "" {
			return errors.New(fmt.Sprintf("Requested image version (%v), doesn't exist", app.Spec.Database.Version))
		}
	}

	makeLocalDB(&dd, nn, app, &dbCfg, image, db.Env.Spec.Providers.Database.PVC)

	if err = exists.Apply(db.Ctx, db.Client, &dd); err != nil {
		return err
	}

	s := core.Service{}
	update, err := utils.UpdateOrErr(db.Client.Get(db.Ctx, nn, &s))

	if err != nil {
		return err
	}

	makeLocalService(&s, nn, app)

	if err = update.Apply(db.Ctx, db.Client, &s); err != nil {
		return err
	}

	if db.Env.Spec.Providers.Database.PVC {
		pvc := core.PersistentVolumeClaim{}
		update, err = utils.UpdateOrErr(db.Client.Get(db.Ctx, nn, &pvc))

		if err != nil {
			return err
		}

		makeLocalPVC(&pvc, nn, app)

		if err = update.Apply(db.Ctx, db.Client, &pvc); err != nil {
			return err
		}
	}
	c.Database = &db.Config
	return nil
}

func makeLocalDB(dd *apps.Deployment, nn types.NamespacedName, app *crd.ClowdApp, cfg *config.DatabaseConfig, image string, usePVC bool) {
	labels := app.GetLabels()
	labels["service"] = "db"
	labler := utils.MakeLabeler(nn, labels, app)
	labler(dd)

	var volSource core.VolumeSource
	if usePVC {
		volSource = core.VolumeSource{
			PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
				ClaimName: nn.Name,
			},
		}
	} else {
		volSource = core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		}
	}

	if app.Spec.Cyndi {
		cmData := make(map[string]string)
		setUpUsersSh := `
			psql $POSTGRESQL_DATABASE < /opt/app-root/src/postgresql-start/setUpUsers.sql
			psql $POSTGRESQL_DATABASE -c "GRANT cyndi_reader to $POSTGRESQL_USER"
			psql $POSTGRESQL_DATABASE -c "ALTER USER insightsapi WITH SUPERUSER;"
		`
		setUpUsersSql := `
			CREATE SCHEMA inventory;
			-- The admin ROLE that allows the inventory schema to be managed
			CREATE ROLE cyndi_admin;
			GRANT ALL PRIVILEGES ON SCHEMA inventory TO cyndi_admin;
			-- The reader ROLE that provides SELECT access to the inventory.hosts view
			CREATE ROLE cyndi_reader;
			GRANT USAGE ON SCHEMA inventory TO cyndi_reader;
			-- Create a user for Cyndi with cyndi_admin ROLE
			-- Used by Project Cyndi to access the database and manage the inventory schema
			CREATE USER cyndi WITH PASSWORD 'cyndi' IN ROLE cyndi_admin;
		`
		cmData["setUpUsers.sh"] = setUpUsersSh
		cmData["setUpUsersSql"] = setUpUsersSql
		cm := core.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      nn.Name + "-init",
				Namespace: nn.Namespace,
			},
			Data: cmData,
		}
	}

	dd.Spec.Replicas = utils.Int32(1)
	dd.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	dd.Spec.Template.Spec.Volumes = []core.Volume{
		{
			Name:         nn.Name,
			VolumeSource: volSource,
		},
	}
	dd.Spec.Template.ObjectMeta.Labels = labels

	dd.Spec.Template.Spec.ImagePullSecrets = []core.LocalObjectReference{{
		Name: "quay-cloudservices-pull",
	}}

	envVars := []core.EnvVar{
		{Name: "POSTGRESQL_USER", Value: cfg.Username},
		{Name: "POSTGRESQL_PASSWORD", Value: cfg.Password},
		{Name: "PGPASSWORD", Value: cfg.AdminPassword}, // Legacy for old db images can likely be removed soon
		{Name: "POSTGRESQL_MASTER_USER", Value: cfg.AdminUsername},
		{Name: "POSTGRESQL_MASTER_PASSWORD", Value: cfg.AdminPassword},
		// TODO: Do we need to set the DB name?
		{Name: "POSTGRESQL_DATABASE", Value: app.Spec.Database.Name},
	}
	ports := []core.ContainerPort{{
		Name:          "database",
		ContainerPort: 5432,
	}}

	probeHandler := core.Handler{
		Exec: &core.ExecAction{
			Command: []string{
				"psql",
				"-U",
				"$(POSTGRESQL_USER)",
				"-d",
				"$(POSTGRESQL_DATABASE)",
				"-c",
				"SELECT 1",
			},
		},
	}

	livenessProbe := core.Probe{
		Handler:             probeHandler,
		InitialDelaySeconds: 15,
		TimeoutSeconds:      2,
	}
	readinessProbe := core.Probe{
		Handler:             probeHandler,
		InitialDelaySeconds: 45,
		TimeoutSeconds:      2,
	}

	c := core.Container{
		Name:           nn.Name,
		Image:          image,
		Env:            envVars,
		LivenessProbe:  &livenessProbe,
		ReadinessProbe: &readinessProbe,
		Ports:          ports,
		VolumeMounts: []core.VolumeMount{{
			Name:      nn.Name,
			MountPath: "/var/lib/pgsql/data",
		}},
	}

	dd.Spec.Template.Spec.Containers = []core.Container{c}
}

func makeLocalService(s *core.Service, nn types.NamespacedName, app *crd.ClowdApp) {
	servicePorts := []core.ServicePort{{
		Name:     "database",
		Port:     5432,
		Protocol: "TCP",
	}}
	utils.MakeService(s, nn, p.Labels{"service": "db", "app": app.Name}, servicePorts, app)
}

func makeLocalPVC(pvc *core.PersistentVolumeClaim, nn types.NamespacedName, app *crd.ClowdApp) {
	utils.MakePVC(pvc, nn, p.Labels{"service": "db", "app": app.Name}, "1Gi", app)
}
