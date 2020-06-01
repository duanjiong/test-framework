#!/usr/bin/env bash

IMG="duanjiong/gobgp:latest"

docker build -f doc-yaml/gobgp.Dockerfile -t ${IMG} .
docker push ${IMG}
