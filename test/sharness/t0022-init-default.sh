#!/usr/bin/env bash
#
# Copyright (c) 2014 Christian Couder
# MIT Licensed; see the LICENSE file in this repository.
#

test_description="Test init command with default config"

. lib/test-lib.sh

cfg_key="Addresses.API"
cfg_val="/ip4/0.0.0.0/tcp/5001"

# test that init succeeds
test_expect_success "btfs init succeeds" '
  export BTFS_PATH="$(pwd)/.btfs" &&
  echo "BTFS_PATH: \"$BTFS_PATH\"" &&
  BITS="2048" &&
  btfs init --bits="$BITS" >actual_init ||
  test_fsh cat actual_init
'

test_expect_success ".btfs/config has been created" '
  test -f "$BTFS_PATH"/config ||
  test_fsh ls -al .btfs
'

test_expect_success "btfs config succeeds" '
  btfs config $cfg_flags "$cfg_key" "$cfg_val"
'

test_expect_success "btfs read config succeeds" '
  BTFS_DEFAULT_CONFIG=$(cat "$BTFS_PATH"/config)
'

test_expect_success "clean up btfs dir" '
  rm -rf "$BTFS_PATH"
'

test_expect_success "btfs init default config succeeds" '
  echo $BTFS_DEFAULT_CONFIG | btfs init - >actual_init ||
  test_fsh cat actual_init
'

test_expect_success "btfs config output looks good" '
  echo "$cfg_val" >expected &&
  btfs config "$cfg_key" >actual &&
  test_cmp expected actual
'

test_done
