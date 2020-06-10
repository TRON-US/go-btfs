#!/usr/bin/env bash
#
# Copyright (c) 2014 Christian Couder
# MIT Licensed; see the LICENSE file in this repository.
#

test_description="Test init command"

. lib/test-lib.sh

# test that btfs fails to init if BTFS_PATH isnt writeable
test_expect_success "create dir and change perms succeeds" '
  export BTFS_PATH="$(pwd)/.badbtfs" &&
  mkdir "$BTFS_PATH" &&
  chmod 000 "$BTFS_PATH"
'

test_expect_success "btfs init fails" '
  test_must_fail btfs init 2> init_fail_out
'

# Under Windows/Cygwin the error message is different,
# so we use the STD_ERR_MSG prereq.
if test_have_prereq STD_ERR_MSG; then
  init_err_msg="Error: error loading plugins: open $BTFS_PATH/config: permission denied"
else
  init_err_msg="Error: error loading plugins: open $BTFS_PATH/config: The system cannot find the path specified."
fi

test_expect_success "btfs init output looks good" '
  echo "$init_err_msg" >init_fail_exp &&
  test_cmp init_fail_exp init_fail_out
'

test_expect_success "cleanup dir with bad perms" '
  chmod 775 "$BTFS_PATH" &&
  rmdir "$BTFS_PATH"
'

# test no repo error message
# this applies to `btfs add sth`, `ipfs refs <hash>`
test_expect_success "btfs cat fails" '
  export BTFS_PATH="$(pwd)/.btfs" &&
  test_must_fail btfs cat Qmaa4Rw81a3a1VEx4LxB7HADUAXvZFhCoRdBzsMZyZmqHD 2> cat_fail_out
'

test_expect_success "btfs cat no repo message looks good" '
  echo "Error: no BTFS repo found in $BTFS_PATH." > cat_fail_exp &&
  echo "please run: '"'"'btfs init'"'"'" >> cat_fail_exp &&
  test_path_cmp cat_fail_exp cat_fail_out
'

# test that init succeeds
test_expect_success "btfs init succeeds" '
  export BTFS_PATH="$(pwd)/.btfs" &&
  echo "BTFS_PATH: \"$BTFS_PATH\"" &&
  BITS="2048" &&
  btfs init --bits="$BITS" >actual_init ||
  test_fsh cat actual_init
'

test_expect_success ".btfs/ has been created" '
  test -d ".btfs" &&
  test -f ".btfs/config" &&
  test -d ".btfs/datastore" &&
  test -d ".btfs/blocks" &&
  test ! -f ._check_writeable ||
  test_fsh ls -al .btfs
'

test_expect_success "btfs config succeeds" '
  echo /btfs >expected_config &&
  btfs config Mounts.IPFS >actual_config &&
  test_cmp expected_config actual_config
'

test_expect_success "btfs peer id looks good" '
  PEERID=$(btfs config Identity.PeerID) &&
  test_check_peerid "$PEERID"
'

test_expect_success "btfs init output looks good" '
  STARTFILE="btfs cat /btfs/$HASH_WELCOME_DOCS/readme" &&
  echo "initializing BTFS node at $BTFS_PATH" >expected &&
  echo "generating $BITS-bit ECDSA keypair...done" >>expected &&
  echo "peer identity: $PEERID" >>expected &&
  echo "to get started, enter:" >>expected &&
  printf "\\n\\t$STARTFILE\\n\\n" >>expected &&
  test_cmp expected actual_init
'

test_expect_success "Welcome readme exists" '
  btfs cat /btfs/$HASH_WELCOME_DOCS/readme
'

test_expect_success "clean up btfs dir" '
  rm -rf "$BTFS_PATH"
'

test_expect_success "'btfs init --empty-repo' succeeds" '
  BITS="2048" &&
  btfs init --bits="$BITS" --empty-repo >actual_init
'

test_expect_success "btfs peer id looks good" '
  PEERID=$(btfs config Identity.PeerID) &&
  test_check_peerid "$PEERID"
'

test_expect_success "'btfs init --empty-repo' output looks good" '
  echo "initializing BTFS node at $BTFS_PATH" >expected &&
  echo "generating $BITS-bit ECDSA keypair...done" >>expected &&
  echo "peer identity: $PEERID" >>expected &&
  test_cmp expected actual_init
'

test_expect_success "Welcome readme doesn't exists" '
  test_must_fail btfs cat /btfs/$HASH_WELCOME_DOCS/readme
'

test_expect_success "btfs id agent string contains correct version" '
  btfs id -f "<aver>" | grep $(btfs version -n)
'

test_expect_success "clean up btfs dir" '
  rm -rf "$BTFS_PATH"
'

# test init profiles
test_expect_success "'btfs init --profile' with invalid profile fails" '
  BITS="2048" &&
  test_must_fail btfs init --bits="$BITS" --profile=nonexistent_profile 2> invalid_profile_out
  EXPECT="Error: invalid configuration profile: nonexistent_profile" &&
  grep "$EXPECT" invalid_profile_out
'

test_expect_success "'btfs init --profile' succeeds" '
  BITS="2048" &&
  btfs init --bits="$BITS" --profile=server
'

test_expect_success "'btfs config Swarm.AddrFilters' looks good" '
  btfs config Swarm.AddrFilters > actual_config &&
  test $(cat actual_config | wc -l) = 22
'

test_expect_success "clean up btfs dir" '
  rm -rf "$BTFS_PATH"
'

test_expect_success "'btfs init --profile=test' succeeds" '
  BITS="2048" &&
  btfs init --bits="$BITS" --profile=test
'

test_expect_success "'btfs config Bootstrap' looks good" '
  btfs config Bootstrap > actual_config &&
  test $(cat actual_config) = "[]"
'

test_expect_success "'btfs config Addresses.API' looks good" '
  btfs config Addresses.API > actual_config &&
  test $(cat actual_config) = "/ip4/127.0.0.1/tcp/0"
'

test_expect_success "btfs init from existing config succeeds" '
  export ORIG_PATH=$BTFS_PATH
  export BTFS_PATH=$(pwd)/.btfs-clone

  btfs init "$ORIG_PATH/config" &&
  btfs config Addresses.API > actual_config &&
  test $(cat actual_config) = "/ip4/127.0.0.1/tcp/0"
'

test_expect_success "clean up btfs clone dir and reset BTFS_PATH" '
  rm -rf "$BTFS_PATH" &&
  export BTFS_PATH=$ORIG_PATH
'

test_expect_success "clean up btfs dir" '
  rm -rf "$BTFS_PATH"
'

test_expect_success "'btfs init --profile=lowpower' succeeds" '
  BITS="2048" &&
  btfs init --bits="$BITS" --profile=lowpower
'

test_expect_success "'btfs config Discovery.Routing' looks good" '
  btfs config Routing.Type > actual_config &&
  test $(cat actual_config) = "dhtclient"
'

test_expect_success "clean up btfs dir" '
  rm -rf "$BTFS_PATH"
'

test_init_btfs

test_launch_btfs_daemon

test_expect_success "btfs init should not run while daemon is running" '
  test_must_fail btfs init 2> daemon_running_err &&
  EXPECT="Error: btfs daemon is running. please stop it to run this command" &&
  grep "$EXPECT" daemon_running_err
'

test_kill_btfs_daemon

test_done
