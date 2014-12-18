#!/bin/bash

. helpers.sh

AGENT_PORT=$(random_int 7000 50000)
AGENT_URL="http://127.0.0.1:$AGENT_PORT"

SCHEDULER_PORT=$(random_int 4000 50000)
SCHEDULER_URL="http://127.0.0.1:$SCHEDULER_PORT"

set -e
cd $(dirname $0)

httpdir=/tmp/http.$$.$RANDOM
mkdir $httpdir
( cd ../warhead; make pkg ) > /dev/null 2>&1
cp ../warhead/warhead.tgz $httpdir
trap "rm -rf $httpdir" EXIT

DOWNLOAD_PORT=$(random_int 8090 50000)
(cd $httpdir; python -m SimpleHTTPServer $DOWNLOAD_PORT &)
# Wait up to six seconds for server to start up
./retry 20 0.3 netcat -z localhost $DOWNLOAD_PORT ||
  abort "could not start web server"
trap "nuke 'SimpleHTTPServer $DOWNLOAD_PORT' && rm -rf $httpdir" EXIT

WARHEAD_URL=http://127.0.0.1:${DOWNLOAD_PORT}/warhead.tgz

type nsinit &>/dev/null || {
  echo "nsinit executable not available" >&2
  echo "run: go install github.com/docker/libcontainer/nsinit" >&2
  exit 1
}
nsinit=$(which nsinit)

rootfs=/tmp/rootfs.$$.$RANDOM
rundir=$rootfs/run

echo "run: creating rootfs at $rootfs"
make_rootfs $rootfs || abort "run: unable to create rootfs"
trap "nuke 'SimpleHTTPServer $DOWNLOAD_PORT' && rm -rf $httpdir && rm -rf $rootfs" EXIT

install -D $(which svlogd) $rootfs/bin/

echo "run: install harpoon binaries"
GOBIN=$rootfs/bin go install github.com/soundcloud/harpoon/...

echo "run: install libcontainer config"
go run config.go -rootfs /tmp/rootfs >$rootfs/container.json ||
  abort "run: failed to generate config"

logfile=$PWD/agent.log

echo "run: starting agent at localhost:${AGENT_PORT}"
{
  pushd $rootfs >/dev/null
  sudo $nsinit exec -- $rootfs/bin/harpoon-agent -run ${rundir} -addr ":${AGENT_PORT}" > $logfile 2>&1 & AGENT_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-agent"
trap "nuke 'SimpleHTTPServer $DOWNLOAD_PORT' && rm -rf $httpdir && rm -rf $rootfs && shutdown $AGENT_PID" EXIT

echo "run: waiting for agent to start"
./retry 5 1 curl -s ${AGENT_URL}/api/v0/containers || abort "could not start agent"

echo "==== Agent: Basic Tests ===="
go test -v agent-test-basic/basic_test.go -integ.agent.url=${AGENT_URL} -integ.warhead.url=${WARHEAD_URL}

echo "==== Agent: Failed Creation Tests ===="
go test -v agent-test-basic/failed_creation_test.go -integ.agent.url=${AGENT_URL}

echo $logfile

logfile=$PWD/scheduler.log

echo "run: starting scheduler at localhost:${SCHEDULER_PORT}"
{
  pushd $rootfs >/dev/null
  harpoon-scheduler -listen=":${SCHEDULER_PORT}" -agent=${AGENT_URL} > $logfile 2>&1  & SCHEDULER_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-scheduler"
trap "nuke 'SimpleHTTPServer $DOWNLOAD_PORT' && rm -rf $httpdir && rm -rf $rootfs && shutdown $AGENT_PID && shutdown $SCHEDULER_PID" EXIT

echo "run: waiting for scheduler to start"
./retry 5 1 curl -s ${SCHEDULER_URL}/ || abort "could not start scheduler"

# These scheduler tests are *completely* broken.  They won't even build.  This problem is Issue #137.
echo "==== Scheduler: Basic Tests ===="
echo "**** SCHEDULER TESTS ARE CURRENTLY DISABLED DUE TO CODE ROT. SEE ISSUE #137 ****"
# go test -v scheduler-test-basic/basic_test.go -integ.scheduler.url=${SCHEDULER_URL} -integ.agent.url=${AGENT_URL}

echo $logfile
