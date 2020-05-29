#!/usr/bin/env bash
#ref https://github.com/kubernetes/kubernetes/issues/79384

echo 'require k8s.io/kubernetes 8c3b7d7679ccf368'
echo 'require ('
curl -sS https://raw.githubusercontent.com/kubernetes/kubernetes/8c3b7d7679ccf368/go.mod \
| sed -n -r 's# \./staging/(.*)$# k8s.io/kubernetes/staging/\1 8c3b7d7679ccf368#p' | awk '{print $1 "  v0.0.0"}'
echo ')'
echo 'replace ('
curl -sS https://raw.githubusercontent.com/kubernetes/kubernetes/8c3b7d7679ccf368/go.mod \
| sed -n -r 's# \./staging/(.*)$# k8s.io/kubernetes/staging/\1 8c3b7d7679ccf368#p'
echo ')'



