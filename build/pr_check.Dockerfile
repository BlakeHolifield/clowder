FROM fedora:34
RUN dnf install -y openssh-clients git podman make which go jq
RUN mkdir /root/go/src -p
RUN cd /root/go/src/ \
    && GO111MODULE=on go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1 \
    && GO111MODULE=on go get sigs.k8s.io/kustomize/kustomize/v3@v3.8.7 \
    && rm -rf /root/go/src \
    && rm -rf /root/go/pkg
RUN ln -s /usr/bin/podman /usr/bin/docker
COPY pr_check_inner.sh .
RUN chmod 775 pr_check_inner.sh
