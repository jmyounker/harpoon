#!/bin/bash

# abort prints its arguments to stderr and exits with a status of 1.
function abort {
  echo $* >&2
  exit 1
}

# copy_dependencies installs the dynamic dependencies of bin into dest.
function copy_dependencies {
  bin=$1
  dest=$2

  deps=$(ldd $bin | egrep -o '/[^ ]+')

  for dep in $deps
  do
    install -D $dep $dest/$dep
  done
}

# random_int chooses a random integer in the range [$start, $end)
function random_int {
    local start end size number
    start=$1
    end=$2
    size=$(($end - $start))
    ra=$(($RANDOM % $size))
    echo $(($ra + $start))
}

# make_rootfs makes the provided directory a minimal busybox rootfs.
function make_rootfs {
  echo "making rootfs"
  local rootfs=$1

  mkdir -p \
    $rootfs/bin \
    $rootfs/dev \
    $rootfs/etc \
    $rootfs/proc \
    $rootfs/run \
    $rootfs/sys \
    $rootfs/tmp

  touch \
    $rootfs/etc/hostname \
    $rootfs/etc/resolv.conf
}

function nuke {
  local ptrn found
  ptrn=$1
  found=$(ps wwaux | grep "$ptrn" | grep -v grep | awk '{print $2}' | xargs echo)
  if [ ${#found} -ne 0 ]; then
    echo $found | xargs kill
  fi
}

# shutdown sends SIGTERM and waits for the process to exit. If it takes longer
# than 5 seconds, it is sent SIGKILL
function shutdown {
  local pid found
  pid=$1

  sudo kill -SIGTERM $pid

  # Can't do the sensible thing with wait because nsinit is detached from
  # our process.
  for i in 1 2 3 5; do
    found=$(ps wwaux | awk '{print $2}' | grep "^${pid}$" | wc -l)
    if [ $found -eq 0 ]; then
      return
    fi
    sleep 1
  done

  kill SIGKILL $pid
}
