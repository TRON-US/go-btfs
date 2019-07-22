#!/usr/bin/env bash
#
# Copyright (c) 2014 Christian Couder
# MIT Licensed; see the LICENSE file in this repository.
#

test_description="Test installation and some basic commands"

. lib/test-lib.sh

test_expect_success "current dir is writable" '
  echo "It works!" >test.txt
'

test_expect_success "btfs version succeeds" '
  btfs version >version.txt
'

test_expect_success "btfs --version success" '
  btfs --version
'

test_expect_success "btfs version output looks good" '
  egrep "^btfs version [0-9]+\.[0-9]+\.[0-9]" version.txt >/dev/null ||
  test_fsh cat version.txt
'

test_expect_success "btfs versions matches btfs --version" '
  btfs version > version.txt &&
  btfs --version > version2.txt &&
  diff version2.txt version.txt ||
  test_fsh btfs --version

'

test_expect_success "btfs version --all has all required fields" '
  btfs version --all > version_all.txt &&
  grep "go-btfs version" version_all.txt &&
  grep "Repo version" version_all.txt &&
  grep "System version" version_all.txt &&
  grep "Golang version" version_all.txt
'

test_expect_success "btfs version deps succeeds" '
  btfs version deps >deps.txt
'

test_expect_success "btfs version deps output looks good" '
  head -1 deps.txt | grep "go-btfs@(devel)" &&
  [[ $(tail -n +2 deps.txt | egrep -v -c "^[^ @]+@v[^ @]+( => [^ @]+@v[^ @]+)?$") -eq 0 ]] ||
  test_fsh cat deps.txt
'

test_expect_success "'btfs commands' succeeds" '
  btfs commands >commands.txt
'

test_expect_success "'btfs commands' output looks good" '
  grep "btfs add" commands.txt &&
  grep "btfs daemon" commands.txt
  #grep "btfs update" commands.txt
'

test_expect_success "All sub-commands accept help" '
  echo 0 > fail
  while read -r cmd
  do
    ${cmd:0:4} help ${cmd:5} >/dev/null ||
      { echo "$cmd doesnt accept --help"; echo 1 > fail; }
    echo stuff | $cmd --help >/dev/null ||
      { echo "$cmd doesnt accept --help when using stdin"; echo 1 > fail; }
  done <commands.txt

  if [ $(cat fail) = 1 ]; then
    return 1
  fi
'

test_expect_success "All commands accept --help" '
  echo 0 > fail
  while read -r cmd
  do
    $cmd --help >/dev/null ||
      { echo "$cmd doesnt accept --help"; echo 1 > fail; }
    echo stuff | $cmd --help >/dev/null ||
      { echo "$cmd doesnt accept --help when using stdin"; echo 1 > fail; }
  done <commands.txt

  if [ $(cat fail) = 1 ]; then
    return 1
  fi
'

test_expect_failure "All btfs root commands are mentioned in base helptext" '
  echo 0 > fail
  btfs --help > help.txt
  cut -d" " -f 2 commands.txt | grep -v btfs | sort -u | \
  while read cmd
  do
    grep "  $cmd" help.txt > /dev/null ||
      { echo "missing $cmd from helptext"; echo 1 > fail; }
  done

  if [ $(cat fail) = 1 ]; then
    return 1
  fi
'

test_expect_failure "All btfs commands docs are 80 columns or less" '
  echo 0 > fail
  while read cmd
  do
    LENGTH="$($cmd --help | awk "{ print length }" | sort -nr | head -1)"
    [ $LENGTH -gt 80 ] &&
      { echo "$cmd help text is longer than 79 chars ($LENGTH)"; echo 1 > fail; }
  done <commands.txt

  if [ $(cat fail) = 1 ]; then
    return 1
  fi
'

test_expect_success "All btfs commands fail when passed a bad flag" '
  echo 0 > fail
  while read -r cmd
  do
    test_must_fail $cmd --badflag >/dev/null 2>&1 ||
      { echo "$cmd exit with code 0 when passed --badflag"; echo 1 > fail; }
  done <commands.txt

  if [ $(cat fail) = 1 ]; then
    return 1
  fi
'

test_expect_success "'btfs commands --flags' succeeds" '
  btfs commands --flags >commands.txt
'

test_expect_success "'btfs commands --flags' output looks good" '
  grep "btfs pin add --recursive / btfs pin add -r" commands.txt &&
  grep "btfs id --format / btfs id -f" commands.txt &&
  grep "btfs repo gc --quiet / btfs repo gc -q" commands.txt
'



test_done
