#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright © 2021 Roberto Hidalgo <coredns-consul@un.rob.mx>
@milpa.load_util tmp

root="$(dirname "$MILPA_COMMAND_REPO")"
build=""
@tmp.dir build
read -r source version < <(awk '/coredns\/coredns/ {print $1, $2}' "$root/go.mod") || @milpa.fail "Could not fetch version from go.mod"

ourVersion=$(git describe --tags 2>/dev/null || git describe --always)

rm -rf "$root/dist"
mkdir "$root/dist"
repo="git@${source/.com/.com:}.git"
@milpa.log info "cloning $repo at $version"

git clone --depth 1 --branch "$version" "$repo" "$build" || @milpa.fail "Could not fetch version $version"
cd "$build" || @milpa.fail "Could not cd into cloned coredns dir"

@milpa.log info "Preparing plugins..."
sed -e '/^go /a\
replace github.com\/unRob\/coredns-consul => '"$root" go.mod > go.mod.new
mv go.mod.new go.mod

sed \
  -e 's/kubernetes:kubernetes/consul_catalog:github.com\/unRob\/coredns-consul/;' \
  -e '/route53:route53/d;' \
  -e '/azure:azure/d;' \
  -e '/clouddns:clouddns/d;' \
  -e '/k8s_external:k8s_external/d;' \
  -e '/kubernetes:kubernetes/d;' \
  -e '/etcd:etcd/d;' \
  plugin.cfg > plugin.cfg.new || @milpa.fail "Could not configure plugins"

mv plugin.cfg.new plugin.cfg

go generate
go get
@milpa.log success "plugins configured"

for osarch in "${MILPA_ARG_TARGETS[@]}"; do
  os="${osarch%/*}"
  arch="${osarch#*/}"
  @milpa.log info "Building for $osarch"
  binary="$root/dist/coredns"
  package="$root/dist/coredns-consul-${osarch////-}.tgz"

  make coredns \
    SYSTEM="GOARCH=$arch GOOS=$os" \
    GITCOMMIT="$(git describe --always)+consul-v$ourVersion" \
    BINARY="$binary" || @milpa.fail "could not build for $osarch"
  tar -czf "$package" -C "$root/dist" coredns || @milpa.fail "Could not archive $package"
  rm "$binary"
  @milpa.log success "Built for $osarch at $package"
done
