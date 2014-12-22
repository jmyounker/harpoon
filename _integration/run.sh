#!/bin/bash

# Normally set to zero so that the integration tests clean up after themselves.
#
# Set to non-zero if you want examine artifacts and processes.
KEEP_ARTIFACTS=0

. helpers.sh

AGENT_PORT=$(random_int 7000 50000)
AGENT_URL="http://127.0.0.1:$AGENT_PORT"

AGENT_LOG_PORT=$(random_int 3000 50000)

SCHEDULER_PORT=$(random_int 4000 50000)
SCHEDULER_URL="http://127.0.0.1:$SCHEDULER_PORT"

set -e
cd $(dirname $0)

# set up artifact download directories
httpdir=/tmp/http.$$.$RANDOM
mkdir $httpdir

# build and install artifact into download directories
pushd ../warhead >/dev/null
make pkg > /dev/null 2>&1
popd >/dev/null
cp ../warhead/warhead.tgz $httpdir
if [ $KEEP_ARTIFACTS -eq 0 ]; then
    trap "rm -rf $httpdir" EXIT
fi

# start up http server for artifacts
DOWNLOAD_PORT=$(random_int 8090 50000)
pushd $httpdir >/dev/null
python -m SimpleHTTPServer $DOWNLOAD_PORT & HTTPD_PID=$!
popd
# Wait up to six seconds for server to start up
./retry 20 0.3 netcat -z localhost $DOWNLOAD_PORT ||
  abort "could not start web server"
if [ $KEEP_ARTIFACTS -eq 0 ]; then
   trap "shutdown $HTTPD_PID && rm -rf $httpdir" EXIT
fi

WARHEAD_URL=http://127.0.0.1:${DOWNLOAD_PORT}/warhead.tgz

# nsinit runs process in a container container
type nsinit &>/dev/null || {
  go install github.com/docker/libcontainer/nsinit || abort "could not install nsinit"
}
nsinit=$(which nsinit)

# build the rootfs
rootfs=/tmp/rootfs.$$.$RANDOM
echo "run: creating rootfs at $rootfs"
make_rootfs $rootfs || abort "run: unable to create rootfs"
if [ $KEEP_ARTIFACTS -eq 0 ]; then
  trap "shutdown $HTTPD_PID && rm -rf $httpdir && sudo rm -rf $rootfs" EXIT
fi

# svlogd is necessary for collecting logs
install -D $(which svlogd) $rootfs/bin/
# tar is necessary for unpacking downloaded artifacts
install -D $(which tar) $rootfs/bin/
# tar uses these programs to uncompress files
install -D $(which bunzip2) $rootfs/bin/
install -D $(which gunzip) $rootfs/bin/
install -D $(which gzip) $rootfs/bin/

# Debugging tools
if [ $KEEP_ARTIFACTS -ne 0 ]; then
  install -D $(which bash) $rootfs/bin/
  install -D $(which cat) $rootfs/bin/
  install -D $(which curl) $rootfs/bin/
  install -D $(which ls) $rootfs/bin/
  install -D $(which mkdir) $rootfs/bin/
  install -D $(which rm) $rootfs/bin/
  install -D $(which telnet) $rootfs/bin/
  echo "======= Debug within container using: pushd $rootfs; sudo $nsinit exec /bin/bash; popd"
fi

echo "run: install harpoon binaries"
GOBIN=$rootfs/srv/harpoon/bin go install github.com/soundcloud/harpoon/...

# install dependencies for everything that runs in the container
echo "run: install harpoon libraries"
for bin in $rootfs/srv/harpoon/bin/* $rootfs/bin/*
do
  copy_dependencies $bin $rootfs ||
    abort "run: unable to install dependencies"
done

echo "run: install libcontainer config"
# The -rootfs path is the absolute path for the container's root. It is from the perspective
# of this script.
go run config.go -rootfs $rootfs >$rootfs/container.json ||
  abort "run: failed to generate config"

logfile=$PWD/agent.log

echo "run: starting agent at localhost:${AGENT_PORT}"
{
  # Change into the $rootfs. Nsinit will configure itself from ./container.json
  pushd $rootfs >/dev/null
  # The path to harpoon-agent is the absolute path *within* the container, as is /run.
  sudo $nsinit exec -- /srv/harpoon/bin/harpoon-agent -addr ":${AGENT_PORT}" -log.addr ":${AGENT_LOG_PORT}" -download.timeout=20s -debug > $logfile 2>&1  & AGENT_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-agent"
if [ $KEEP_ARTIFACTS -eq 0 ]; then
  trap "shutdown $AGENT_PID && shutdown $HTTPD_PID && rm -rf $httpdir && sudo rm -rf $rootfs" EXIT
fi

echo "run: waiting for agent to start"
./retry 5 1 curl -s ${AGENT_URL}/api/v0/containers || abort "could not start agent"

echo "==== Agent: Basic Tests ===="
go test -v agent-test-basic/basic_test.go -integ.agent.url=${AGENT_URL} -integ.warhead.url=${WARHEAD_URL}

echo "==== Agent: Failed Creation Tests ===="
# go test -v agent-test-basic/failed_creation_test.go -integ.agent.url=${AGENT_URL}

echo "agent logfile: $logfile"

logfile=$PWD/scheduler.log

echo "run: starting scheduler at localhost:${SCHEDULER_PORT}"
{
  pushd $rootfs >/dev/null
  # The path to harpoon-scheduler is the absolute path *within* the container
  sudo $nsinit exec -- /srv/harpoon/bin/harpoon-scheduler -listen=":${SCHEDULER_PORT}" -agent=${AGENT_URL} > $logfile 2>&1  & SCHEDULER_PID=$!
  popd >/dev/null
} || abort "unable to start harpoon-scheduler"
if [ $KEEP_ARTIFACTS -eq 0 ]; then
  trap "shutdown $SCHEDULER_PID && shutdown $AGENT_PID && shutdown $HTTPED_PID && rm -rf $httpdir && sudo rm -rf $rootfs" EXIT
fi

echo "run: waiting for scheduler to start"
./retry 5 1 curl -s ${SCHEDULER_URL}/ || abort "could not start scheduler"

# These scheduler tests are *completely* broken.  They won't even build.  This problem is Issue #137.
echo "==== Scheduler: Basic Tests ===="
echo "**** SCHEDULER TESTS ARE CURRENTLY DISABLED DUE TO CODE ROT. SEE ISSUE #137 ****"
# go test -v scheduler-test-basic/basic_test.go -integ.scheduler.url=${SCHEDULER_URL} -integ.agent.url=${AGENT_URL}

echo "scheduler logfile: $logfile"
