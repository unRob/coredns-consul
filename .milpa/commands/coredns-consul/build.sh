#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright © 2021 Roberto Hidalgo <coredns-consul@un.rob.mx>
@milpa.load_util tmp

if [[ "${MILPA_ARG_TARGETS[0]}" == "all" ]]; then
  IFS=$'\n' read -rd '' -a MILPA_ARG_TARGETS < <(
    milpa itself command-tree coredns-consul --output json --silent |
      jq -r '.children | map(select(.command.path | last == "build")) | first | .command.arguments[0].values.static | map(select(. != "all"))[]'
  )
fi

root="$(dirname "$MILPA_COMMAND_REPO")"
build=""
@tmp.dir build
read -r coredns version < <(awk '/coredns\/coredns/ {sub(".com", ".com:", $1); print "git@" $1".git", $2}' "$root/go.mod") || @milpa.fail "Could not fetch version from go.mod"
plugin=$(git remote -v | awk '{ gsub(/(\.git|git@|https:\/\/)/, ""); gsub(":/", "/"); print $2; exit }')

ourVersion=$(git describe --tags 2>/dev/null || git describe --always)

rm -rf "$root/dist"
mkdir "$root/dist"
@milpa.log info "cloning $coredns at $version"

git -c advice.detachedHead=false clone --depth 1 --branch "$version" "$coredns" "$build" || @milpa.fail "Could not fetch version $version"
cd "$build" || @milpa.fail "Could not cd into cloned coredns dir"

@milpa.log info "Adding plugin: $plugin..."
sed -e '/^go /a\
replace '"$plugin"' => '"$root" go.mod > go.mod.new
mv go.mod.new go.mod

@milpa.log info "Generating build configuration files..."
sed \
  -e 's|kubernetes:kubernetes|consul_catalog:'"$plugin"'|;' \
  -e '/route53:route53/d;' \
  -e '/azure:azure/d;' \
  -e '/clouddns:clouddns/d;' \
  -e '/k8s_external:k8s_external/d;' \
  -e '/kubernetes:kubernetes/d;' \
  -e '/etcd:etcd/d;' \
  plugin.cfg > plugin.cfg.new || @milpa.fail "Could not configure plugins"

mv plugin.cfg.new plugin.cfg
go generate || @milpa.fail "failed during go generate"
go get || @milpa.fail "failed during go get"
@milpa.log success "coredns is ready to build"

@milpa.log info "Building for platforms: ${MILPA_ARG_TARGETS[*]}"
for osarch in "${MILPA_ARG_TARGETS[@]}"; do
  os="${osarch%/*}"
  arch="${osarch#*/}"
  @milpa.log info "Building for $osarch"
  binary="$root/dist/coredns"
  package="$root/dist/coredns-consul-${osarch////-}.tgz"

  make coredns \
    SYSTEM="GOARCH=$arch GOOS=$os" \
    GITCOMMIT="$(git describe --always)+$plugin@$ourVersion" \
    BINARY="$binary" 2>build.log || { cat build.log; @milpa.fail "could not build for $osarch"; }
  tar -czf "$package" -C "$root/dist" coredns || @milpa.fail "Could not archive $package"
  openssl dgst -sha256 "$package" | awk '{print $2}' > "${package##.tgz}.shasum" || @milpa.fail "Could not generate shasum for $package"

  rm "$binary"
  @milpa.log success "Built for $osarch at $package"
done
