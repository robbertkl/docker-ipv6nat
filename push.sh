#!/bin/sh

BUILDER=ipv6nat-builder
DIST_DIR=dist
VERSION="${1}"

if [ -z "${VERSION}" ]
then
    echo "Usage: ${0} X.X.X"
    exit 1
fi

DOCKER_TLS_VERIFY=
DOCKER_HOST=
DOCKER_CERT_PATH=
DOCKER_MACHINE_NAME=
DOCKER_VERSION="system"

cd `dirname "${0}"`

set -e
set -x

docker buildx rm "${BUILDER}" || true
docker buildx create --name "${BUILDER}" --use
docker buildx build \
    --platform "linux/amd64,linux/arm64,linux/arm/v6,linux/arm/v7" \
    --pull \
    --push \
    --tag "robbertkl/ipv6nat:${VERSION}" \
    --tag "robbertkl/ipv6nat:latest" \
    .

BUILDER_CONTAINER="buildx_buildkit_${BUILDER}0"
docker exec "${BUILDER_CONTAINER}" sh -c \
    'mkdir /dist; mv /var/lib/buildkit/runc-overlayfs/snapshots/snapshots/*/fs/docker-ipv6nat.* /dist'
rm -Rf "${DIST_DIR}/"
docker cp "${BUILDER_CONTAINER}:/dist" "${DIST_DIR}"

docker buildx use default
docker buildx rm "${BUILDER}"
