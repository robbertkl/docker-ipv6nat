#!/bin/sh

cd `dirname "${0}"`

VERSION="${1}"
BUILDER=ipv6nat-builder
PLATFORMS="linux/amd64,linux/arm64,linux/arm/v6,linux/arm/v7"
BUILD_DIR=build
DIST_DIR=dist

if [ -z "${VERSION}" ]
then
    echo "Usage: ${0} X.X.X"
    exit 1
fi

set -e
set -x

docker buildx rm "${BUILDER}" || true
docker buildx create --name "${BUILDER}" --use
docker buildx build \
    --platform "${PLATFORMS}" \
    --pull \
    --build-arg "VERSION=${VERSION}" \
    --output "type=local,dest=${BUILD_DIR}" \
    --push
    --tag "robbertkl/ipv6nat:${VERSION}" \
    --tag "robbertkl/ipv6nat:latest" \
    .
docker buildx use default
docker buildx rm "${BUILDER}"

mkdir -p "${DIST_DIR}"
for PLATFORM in `echo "${PLATFORMS}" | sed 's/,/ /g'`
do
    PLATFORM_BUILD_DIR="${BUILD_DIR}/`echo "${PLATFORM}" | sed -E 's/\//_/g'`"
    PLATFORM_TAG="`echo "${PLATFORM}" | sed -E 's/(^linux|\/)//g'`"
    mv "${PLATFORM_BUILD_DIR}/docker-ipv6nat" "${DIST_DIR}/docker-ipv6nat.${PLATFORM_TAG}"
done
rm -Rf "${BUILD_DIR}/"
