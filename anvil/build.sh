#!/bin/bash
#set -xe
set -e

vers="v0.1"
now=$(date +'%Y-%m-%d_%T')
ldflags="-X main.buildVersion=$vers -X main.buildTime=$now"
go_build_flags=""

function usage() {
  echo "Usage: $0 [-a ARCH] [-o OS] [-t]"
  echo "The supported values fo ARCH are '386' (for 32-bit x86) or 'amd64' (for 64-bit x86_64)"
  echo "The supported values fo OS are 'linux' or 'windows'"
  echo "Each of the -o and -a flags may be specified multiple times to produce builds for multiple architectures"
  echo "The -t option trims the paths in the binaries"
  exit 1
}

GOARCHS=""
GOOSS=""

function parse_opts() {
  while getopts "a:o:t" o
  do
    case "$o" in
      a)
        GOARCHS="$GOARCHS $OPTARG"
        ;;
      o)
        GOOSS="$GOOSS $OPTARG"
        ;;
      t)
        go_build_flags="-trimpath"
        ;;
      *)
        usage
        ;;
    esac
  done
}

function clean() {
  rm -f anvil.exe anvil-con.exe anvil
}

function move_if_exists() {
  local src=$1
  local dst=$2

  if [ -f "$src" ]
  then
    mv $src $dst
  fi
}

function build_anvil() {
  pushd src/anvil > /dev/null

  rm -f anvil anvil.exe anvil-con.exe
  if [ "$GOOS" = "windows" ]
  then
    gogio -ldflags "$ldflags" -icon ../../img/anvil32b.png -buildmode exe -target windows .
    go build -o anvil-con.exe -ldflags "$ldflags" $go_build_flags
  else
    go build -ldflags "$ldflags" $go_build_flags
  fi

  popd > /dev/null

  move_if_exists src/anvil/anvil.exe .
  move_if_exists src/anvil/anvil-con.exe .
  move_if_exists src/anvil/anvil .
}

function build_all() {
  local msg=$1

  if [ "$msg" = "" ]
  then
    msg="native os and arch"
  fi

  echo "Building anvil for $msg"
  build_anvil
}

function build_all_arch() {
  local msg=$1

  if [ "$GOARCHS" = "" ]
  then
    build_all "$msg"
    return
  fi

  for x in $GOARCHS
  do
    if [ "$msg" = "" ]
    then
      msg="arch: $x"
    else
      msg="$msg, arch: $x"
      echo $msg
    fi

    export GOARCH=$x
    build_all "$msg"
  done
}

function build_all_os_and_arch() {
  if [ "$GOOSS" = "" ]
  then
    build_all_arch
    return
  fi

  for x in $GOOSS
  do
    export GOOS=$x
    build_all_arch "os: $x"
  done
}

parse_opts $@

clean

echo "building version $vers"
build_all_os_and_arch
