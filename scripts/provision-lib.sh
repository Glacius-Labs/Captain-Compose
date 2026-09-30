#!/usr/bin/env bash

validate_config_destination() {
  local path=$1 parent
  parent=$(dirname -- "$path")
  if [[ -L "$parent" ]]; then
    echo "Refusing symlink configuration directory: $parent" >&2
    return 1
  fi
  if [[ -L "$path" ]]; then
    echo "Refusing symlink configuration file: $path" >&2
    return 1
  fi
  if [[ -e "$path" && ! -f "$path" ]]; then
    echo "Refusing non-regular configuration file: $path" >&2
    return 1
  fi
}

secure_config_destination() {
  local path=$1 group=$2 owner=${3:-root}
  validate_config_destination "$path" || return
  [[ -f "$path" ]] || { echo "Configuration file does not exist: $path" >&2; return 1; }
  chown -- "$owner:$group" "$path"
  chmod 0640 -- "$path"
}
