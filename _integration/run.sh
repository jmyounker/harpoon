#!/bin/bash

AGENT_PORT="7777"
AGENT_URL="http://127.0.0.1:$AGENT_PORT"

SCHEDULER_PORT="4444"
SCHEDULER_URL="http://127.0.0.1:$SCHEDULER_PORT"

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

[ -d $rootfs ] && {
  echo "run: removing previous rootfs at $rootfs"
  sudo rm -rf $rootfs || abort "run: unable to remove pevious rootfs"
}

echo "run: creating rootfs at $rootfs"
make_rootfs $rootfs || abort "run: unable to create rootfs"

install -D $(which svlogd) $rootfs/bin/

echo "run: install harpoon binaries"
# GOBIN=$rootfs/srv/harpoon/bin go install github.com/soundcloud/harpoon/...
go install github.com/soundcloud/harpoon/...

# echo "run: install harpoon libraries"
# for bin in $rootfs/srv/harpoon/bin/*
# do
#   copy_dependencies $bin $rootfs ||
#     abort "run: unable to install dependencies"
# done

echo "run: install libcontainer config"
go run config.go -rootfs /tmp/rootfs >$rootfs/container.json ||
  abort "run: failed to generate config"

artifact_dir=$rootfs/srv/harpoon/artifacts/asset-host.test/busybox
echo "run: create test artifact"
[ ! -d $artifact_dir ] && {
  make_rootfs $artifact_dir || abort "run: unable to create artifact"
}

logfile=$PWD/agent.log

echo "run: starting agent at localhost:${AGENT_PORT}"
{
  pushd $rootfs >/dev/null
  sudo $nsinit exec -- /srv/harpoon/bin/harpoon-agent -addr ":${AGENT_PORT}" > $logfile 2>&1  & AGENT_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-agent"
trap "shutdown $AGENT_PID" EXIT

echo "run: waiting for agent to start"
for i in {1..5}
do
  if curl -s ${AGENT_URL}/api/v0/containers > /dev/null 2>&1 ; then
    break
  elif [ "$i" -eq "5" ]; then
    abort "run: agent not responsive"
  else
    sleep 1
  fi
done

echo "==== Agent: Basic Tests ===="
go test -v agent-test-basic/basic_test.go -integ.agent.url=${AGENT_URL}

echo "==== Agent: Failed Creation Tests ===="
go test -v agent-test-basic/failed_creation_test.go -integ.agent.url=${AGENT_URL}

echo $logfile

logfile=$PWD/scheduler.log

echo "run: starting scheduler at localhost:${SCHEDULER_PORT}"
{
  pushd $rootfs >/dev/null
  sudo $nsinit exec -- /srv/harpoon/bin/harpoon-scheduler -listen=":${SCHEDULER_PORT}" -agent=${AGENT_URL} > $logfile 2>&1  & SCHEDULER_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-scheduler"
trap "shutdown $SCHEDULER_PID & shutdown $AGENT_PID" EXIT

echo "run: waiting for scheduler to start"
for i in {1..5}
do
  if curl -s ${SCHEDULER_URL}/ > /dev/null 2>&1 ; then
    break
  elif [ "$i" -eq "5" ]; then
    abort "run: scheduler not responsive"
  else
    sleep 1
  fi
done

# These scheduler tests are *completely* broken.  They won't even build.  This problem is Issue #137.
echo "==== Scheduler: Basic Tests ===="
echo "**** SCHEDULER TESTS ARE CURRENTLY DISABLED DUE TO CODE ROT. SEE ISSUE #137 ****"
# go test -v scheduler-test-basic/basic_test.go -integ.scheduler.url=${SCHEDULER_URL} -integ.agent.url=${AGENT_URL}

echo $logfile
