# See here for image contents: https://github.com/microsoft/vscode-dev-containers/tree/v0.191.1/containers/go/.devcontainer/base.Dockerfile

# [Choice] Go version: 1, 1.16, 1.15
ARG VARIANT="1.18"
FROM mcr.microsoft.com/vscode/devcontainers/go:0-${VARIANT}

# [Option] Enable non-root Docker access in container
ARG ENABLE_NONROOT_DOCKER="true"
# [Option] Use the OSS Moby CLI instead of the licensed Docker CLI
ARG USE_MOBY="false"

# [Optional] Uncomment this section to install additional OS packages.
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends bash-completion

# Install kubectl
RUN curl -sSL -o /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl \
    && kubectl completion bash > /etc/bash_completion.d/kubectl

# Replace localhost with host.docker.internal
COPY library-scripts/copy-kube-config.sh /usr/local/share/
RUN echo "source /usr/local/share/copy-kube-config.sh" | tee -a /home/vscode/.bashrc >> /home/vscode/.zshrc

# Configure for Docker access
COPY library-scripts/docker-debian.sh /tmp/library-scripts/
RUN apt-get update \
    && bash /tmp/library-scripts/docker-debian.sh "${ENABLE_NONROOT_DOCKER}" "/var/run/docker-host.sock" "/var/run/docker.sock" "${USERNAME}" "${USE_MOBY}"
ENTRYPOINT ["/usr/local/share/docker-init.sh"]
CMD ["sleep", "infinity"]
