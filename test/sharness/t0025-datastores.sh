#!/usr/bin/env bash

test_description="Test non-standard datastores"

. lib/test-lib.sh

test_expect_success "'btfs init --profile=badgerds' succeeds" '
  BITS="2048" &&
  btfs init --profile=badgerds
'

test_expect_success "'btfs pin ls' works" '
  btfs pin ls | wc -l | grep 9
'

test_done
