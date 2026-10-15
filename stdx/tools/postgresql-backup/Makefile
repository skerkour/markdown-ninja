DIST_DIR = dist
DOCKER_IMAGE = ghcr.io/skerkour/postgresql-backup

.PHONY: build
build:
	mkdir -p $(DIST_DIR)
	GOAMD64=v3 GOOS=linux CGO_ENABLED=0 go build -o  $(DIST_DIR)/postgresql-backup -tags timetzdata \
		-trimpath -a -ldflags "-B none -extldflags -static -w -s" \
		.

.PHONY: docker_build
docker_build:
	docker build -t $(DOCKER_IMAGE):latest -f Dockerfile ../..
