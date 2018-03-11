#!/usr/bin/env bash
set -x

export AWS_DEFAULT_REGION=eu-west-1

SCRIPT_DIR="$( cd "$( dirname "$0" )" && pwd )"

pushd docker
<<<<<<< HEAD
docker-compose run -u ${UID} dev $@
=======
<<<<<<< HEAD
docker-compose run -u $(id -u) dev $@
=======
docker-compose run -u ${UID} dev $@
>>>>>>> 44601e9... adding make clean, launching docker as local user
>>>>>>> adding make clean, launching docker as local user
popd
