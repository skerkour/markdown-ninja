FROM node:slim as node

FROM golang:1.27 AS go

FROM debian:13

ENV TZ="UTC"
RUN echo "${TZ}" > /etc/timezone

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt upgrade -y
RUN apt-get install -y --no-install-recommends \
    # base system. psmisc for killall. openssh-client for git over SSH. netbase for whois.
    bash curl wget psmisc ca-certificates lsb-release openssh-client netbase whois sqlite3 less socat net-tools \
    # dev tools. dnsutils for dig. bsdmainutils for hexdump.
    git vim make binutils coreutils build-essential pkg-config xxd \
    zip gnupg gcc libc6-dev htop wrk zstd gzip upx gdb dnsutils jq b3sum bsdmainutils \
    #  g++ musl musl-dev musl-tools clang libclang-dev lld cmake \
    # useful data. mailcap for mimetypes. tzdata for timezones.
    mailcap tzdata libpcre2-dev \
    # mdninja-server specific dependencies
    postgresql-client-17 libimage-exiftool-perl graphviz

# setup node
COPY --from=node /usr/local/bin/node /usr/local/bin/
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
  && ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

RUN /usr/local/bin/npm i -g npm@latest
RUN npm config set ignore-scripts true
RUN npm config set min-release-age 7


# Setup dev user
ARG USERNAME=dev
ARG USER_UID=10001
ARG USER_GID=$USER_UID

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/home/${USERNAME}" \
    --shell "/bin/bash" \
    --uid "${USER_UID}" \
    "${USERNAME}"


USER $USERNAME

COPY --chown=$USERNAME bashrc /home/$USERNAME/.bashrc
COPY --chown=$USERNAME vimrc /home/$USERNAME/.vimrc

# setup go
RUN mkdir -p /home/$USERNAME/.local/gopath
COPY --from=go /usr/local/go /usr/local/go
RUN /usr/local/go/bin/go telemetry off

# setup git
RUN git config --global push.autoSetupRemote true
RUN git config --global init.defaultBranch main

# setup node (again, as a normal user this time)
RUN npm config set ignore-scripts true
RUN npm config set min-release-age 7
RUN npm config set prefix '~/.npm/'

WORKDIR /
