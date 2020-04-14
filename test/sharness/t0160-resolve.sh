#!/usr/bin/env bash

test_description="Test resolve command"

. lib/test-lib.sh

test_init_ipfs

test_expect_success "resolve: prepare files" '
  mkdir -p a/b &&
  echo "a/b/c" >a/b/c &&
  a_hash=$(ipfs add -q -r a | tail -n1) &&
  b_hash=$(ipfs add -q -r a/b | tail -n1) &&
  c_hash=$(ipfs add -q -r a/b/c | tail -n1)
  a_hash_b32=$(cid-fmt -v 1 -b b %s $a_hash)
  b_hash_b32=$(cid-fmt -v 1 -b b %s $b_hash)
  c_hash_b32=$(cid-fmt -v 1 -b b %s $c_hash)
'

test_expect_success "resolve: prepare dag" '
  dag_hash=$(ipfs dag put <<<"{\"i\": {\"j\": {\"k\": \"asdfasdfasdf\"}}}")
'

test_expect_success "resolve: prepare keys" '
    self_hash=$(ipfs id -f="<id>") &&
    alt_hash=$(ipfs key gen -t rsa alt)
'

test_resolve_setup_name() {
  local key="$1"
  local ref="$2"

  test_expect_success "resolve: prepare $key" '
    ipfs name publish --key="$key" --allow-offline "$ref"
  '
}

test_resolve_setup_name_fail() {
  local key="$1"
  local ref="$2"

  test_expect_failure "resolve: prepare $key" '
    ipfs name publish --key="$key" --allow-offline "$ref"
  '
}

test_resolve() {
  src=$1
  dst=$2
  extra=$3

  test_expect_success "resolve succeeds: $src" '
    ipfs resolve $extra "$src" >actual
  '

  test_expect_success "resolved correctly: $src -> $dst" '
    printf "$dst\n" >expected &&
    test_cmp expected actual
  '
}

test_resolve_cmd() {
  test_resolve "/ipfs/$a_hash" "/ipfs/$a_hash"
  test_resolve "/ipfs/$a_hash/b" "/ipfs/$b_hash"
  test_resolve "/ipfs/$a_hash/b/c" "/ipfs/$c_hash"
  test_resolve "/ipfs/$b_hash/c" "/ipfs/$c_hash"
  test_resolve "/ipld/$dag_hash/i/j/k" "/ipld/$dag_hash/i/j/k"
  test_resolve "/ipld/$dag_hash/i/j" "/ipld/$dag_hash/i/j"
  test_resolve "/ipld/$dag_hash/i" "/ipld/$dag_hash/i"

  test_resolve_setup_name "self" "/ipfs/$a_hash"
  test_resolve "/ipns/$self_hash" "/ipfs/$a_hash"
  test_resolve "/ipns/$self_hash/b" "/ipfs/$b_hash"
  test_resolve "/ipns/$self_hash/b/c" "/ipfs/$c_hash"

  test_resolve_setup_name "self" "/ipfs/$b_hash"
  test_resolve "/ipns/$self_hash" "/ipfs/$b_hash"
  test_resolve "/ipns/$self_hash/c" "/ipfs/$c_hash"

  test_resolve_setup_name "self" "/ipfs/$c_hash"
  test_resolve "/ipns/$self_hash" "/ipfs/$c_hash"

  # simple recursion succeeds
  test_resolve_setup_name "alt" "/ipns/$self_hash"
  test_resolve "/ipns/$alt_hash" "/ipfs/$c_hash"

  # partial resolve succeeds
  test_resolve "/ipns/$alt_hash" "/ipns/$self_hash" -r=false

  # infinite recursion fails
  test_resolve_setup_name "self" "/ipns/$self_hash"
  test_expect_success "recursive resolve terminates" '
    test_expect_code 1 ipfs resolve /ipns/$self_hash 2>recursion_error &&
    grep "recursion limit exceeded" recursion_error
  '
}

test_resolve_cmd_b32() {
  # no flags needed, base should be preserved

  test_resolve "/ipfs/$a_hash_b32" "/ipfs/$a_hash_b32"
  test_resolve "/ipfs/$a_hash_b32/b" "/ipfs/$b_hash_b32"
  test_resolve "/ipfs/$a_hash_b32/b/c" "/ipfs/$c_hash_b32"
  test_resolve "/ipfs/$b_hash_b32/c" "/ipfs/$c_hash_b32"

  # flags needed passed in path does not contain cid to derive base

  test_resolve_setup_name "self" "/ipfs/$a_hash_b32"
  test_resolve "/ipns/$self_hash" "/ipfs/$a_hash_b32" --cid-base=base32
  test_resolve "/ipns/$self_hash/b" "/ipfs/$b_hash_b32" --cid-base=base32
  test_resolve "/ipns/$self_hash/b/c" "/ipfs/$c_hash_b32" --cid-base=base32

  test_resolve_setup_name "self" "/ipfs/$b_hash_b32" --cid-base=base32
  test_resolve "/ipns/$self_hash" "/ipfs/$b_hash_b32" --cid-base=base32
  test_resolve "/ipns/$self_hash/c" "/ipfs/$c_hash_b32" --cid-base=base32

  test_resolve_setup_name "self" "/ipfs/$c_hash_b32"
  test_resolve "/ipns/$self_hash" "/ipfs/$c_hash_b32" --cid-base=base32
}


#todo remove this once the online resolve is fixed
test_resolve_fail() {
  src=$1
  dst=$2

  test_expect_failure "resolve succeeds: $src" '
    ipfs resolve "$src" >actual
  '

  test_expect_failure "resolved correctly: $src -> $dst" '
    printf "$dst" >expected &&
    test_cmp expected actual
  '
}

test_resolve_cmd_fail() {
  test_resolve "/ipfs/$a_hash" "/ipfs/$a_hash"
  test_resolve "/ipfs/$a_hash/b" "/ipfs/$b_hash"
  test_resolve "/ipfs/$a_hash/b/c" "/ipfs/$c_hash"
  test_resolve "/ipfs/$b_hash/c" "/ipfs/$c_hash"
  test_resolve "/ipld/$dag_hash" "/ipld/$dag_hash"
  test_resolve "/ipld/$dag_hash/i/j/k" "/ipld/$dag_hash/i/j/k"
  test_resolve "/ipld/$dag_hash/i/j" "/ipld/$dag_hash/i/j"
  test_resolve "/ipld/$dag_hash/i" "/ipld/$dag_hash/i"

  test_resolve_setup_name_fail "self" "/ipfs/$a_hash"
  test_resolve_fail "/ipns/$self_hash" "/ipfs/$a_hash"
  test_resolve_fail "/ipns/$self_hash/b" "/ipfs/$b_hash"
  test_resolve_fail "/ipns/$self_hash/b/c" "/ipfs/$c_hash"

  test_resolve_setup_name_fail "self" "/ipfs/$b_hash"
  test_resolve_fail "/ipns/$self_hash" "/ipfs/$b_hash"
  test_resolve_fail "/ipns/$self_hash/c" "/ipfs/$c_hash"

  test_resolve_setup_name_fail "self" "/ipfs/$c_hash"
  test_resolve_fail "/ipns/$self_hash" "/ipfs/$c_hash"
}

# should work offline
test_resolve_cmd
test_resolve_cmd_b32

# should work online
test_launch_ipfs_daemon
test_resolve_cmd_fail
test_kill_ipfs_daemon

test_done
