#!/bin/bash

set -e
cd $(dirname $0)

. helpers.sh

type nsinit &>/dev/null || {
  echo "nsinit executable not available" >&2
  echo "run: go install github.com/docker/libcontainer/nsinit" >&2
  exit 1
}

nsinit=$(which nsinit)
rootfs=/tmp/rootfs

echo "run: creating rootfs at $rootfs"
[ ! -d $rootfs ] && {
  make_rootfs $rootfs || abort "run: unable to create rootfs"
}

install -D $(which svlogd) $rootfs/bin/

echo "run: install harpoon binaries"
GOBIN=$rootfs/srv/harpoon/bin go install github.com/soundcloud/harpoon/...

echo "run: install harpoon libraries"
for bin in $rootfs/srv/harpoon/bin/*
do
  copy_dependencies $bin $rootfs ||
    abort "run: unable to install dependencies"
done

echo "run: install libcontainer config"
go run config.go -rootfs /tmp/rootfs >$rootfs/container.json ||
  abort "run: failed to generate config"

artifact_dir=$rootfs/srv/harpoon/artifacts/asset-host.test/busybox
echo "run: create test artifact"
[ ! -d $artifact_dir ] && {
  make_rootfs $artifact_dir || abort "run: unable to create artifact"
}

logfile=$PWD/agent.log
echo "run: starting agent at localhost:7777"
{
  pushd $rootfs >/dev/null
  sudo $nsinit exec -- /srv/harpoon/bin/harpoon-agent -addr ":7777" > $logfile 2>&1  & AGENT_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-agent"
trap "shutdown $AGENT_PID" EXIT

echo "run: waiting for agent to start"
for i in {1..5}
do
  if curl -s http://localhost:7777/containers >/dev/null; then
    break
  elif [ "$i" -eq "5" ]; then
    abort "run: agent not responsive"
  else
    sleep 1
  fi
done

go test -v agent-test-basic/basic_test.go

logfile=$PWD/scheduler.log

echo "run: starting scheduler at localhost:4444"
{
  pushd $rootfs >/dev/null
  sudo $nsinit exec -- /srv/harpoon/bin/harpoon-scheduler -agent=http://127.0.0.1:7777 > $logfile 2>&1  & SCHEDULER_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-scheduler"
trap "shutdown $SCHEDULER_PID & shutdown $AGENT_PID" EXIT

go test -v scheduler-test-basic/basic_test.go

echo $logfile