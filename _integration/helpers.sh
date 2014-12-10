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

# make_rootfs makes the provided directory a minimal busybox rootfs.
function make_rootfs {
  echo "making rootfs"
  local rootfs=$1

  type busybox >/dev/null || {
    echo "busybox executable not available"
    return 1
  }

  file $(which busybox) | grep -q "statically linked" || {
    echo "busybox not statically linked"
    return 1
  }

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

  cp $(which busybox) $rootfs/bin/
  $rootfs/bin/busybox --install .
}

# shutdown sends SIGTERM and waits for the process to exit. If it takes longer
# than 5 seconds, it is sent SIGKILL
function shutdown {
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
