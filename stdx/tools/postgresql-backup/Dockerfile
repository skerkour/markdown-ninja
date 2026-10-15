####################################################################################################
## Builder image
####################################################################################################
FROM golang:1.27 AS builder_go

ENV TZ="UTC"
# ENV LC_ALL="en_US.UTF-8"
# ENV LANG="en_US.UTF-8"
# ENV LANGUAGE="en_US:en"

# Install git + SSL ca certificates.
# Git is required for fetching the dependencies.
# Ca-certificates is required to call HTTPS endpoints.
RUN apt update && apt upgrade -y && \
    apt install -y ca-certificates git make tzdata binutils mailcap
RUN update-ca-certificates


WORKDIR /stdx-go
COPY . ./

RUN mkdir -p dist
RUN GOAMD64=v3 GOOS=linux CGO_ENABLED=0 go build -o dist/postgresql-backup -tags timetzdata \
	-trimpath -a -ldflags "-B none -extldflags -static -w -s" \
	./tools/postgresql-backup/

####################################################################################################
## Final image
####################################################################################################

FROM alpine:latest

ENV TZ="UTC"
ENV LC_ALL="en_US.UTF-8"
ENV LANG="en_US.UTF-8"
ENV LANGUAGE="en_US:en"

RUN apk update && \
    apk upgrade --no-cache && \
    apk add --no-cache ca-certificates postgresql17-client mailcap tzdata && \
    update-ca-certificates && \
    adduser \
        --disabled-password \
        --gecos "" \
        --shell "/bin/nonexisting" \
        --no-create-home \
        --uid 1001 \
        "pgbakcup" && \
    echo "UTC" > /etc/timezone


# Copy our builds
COPY --from=builder_go /stdx-go/dist/postgresql-backup /usr/local/bin/postgresql-backup


# Use an unprivileged user.
USER pgbakcup:pgbakcup

# Final working directory
WORKDIR /app

ENTRYPOINT ["/usr/local/bin/postgresql-backup", "-config", "config.yml"]
