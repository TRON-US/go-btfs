#!/usr/bin/env python
"""Script to find go library dependency"""

import os
import re
import subprocess


# start libs
start_libs = (
        "go-ipfs-config",
        "interface-go-ipfs-core",
        "go-path",
        "go-libp2p",
        "go-multiaddr"
        )


def helper(d, pkg, path):
    if pkg not in d:
        path += " <=== " + pkg
        print path
        return

    if not path:
        path += pkg
    else:
        path += " <=== " + pkg

    for v in d.get(pkg):
        helper(d, v, path)


def print_dep(d):
    print "dependency graph\n"
    for key in start_libs:
        helper(d, key, "")


def run():
    pkgpath = os.environ["GOPATH"] + "/pkg"
    # dependency dictionary
    d = {start_lib: [] for start_lib in start_libs}
    libs = list(start_libs)
    visited_libs = libs

    # continue running when libs is not empty
    while libs:
        next_libs = []
        for lib in libs:
            print lib
            out = subprocess.check_output(
                ["grep", "-nr", lib, pkgpath],
                stderr=subprocess.STDOUT)

            if out:
                lines = out.split("\n")
                for line in lines:
                    # example as below:
                    # ./mod/github.com/libp2p/go-libp2p-kad-dht@v0.0.10/pb/message.go:9:	ma "github.com/multiformats/go-multiaddr"
                    ss = line.split(":")
                    if not ss or len(ss) < 2:
                        continue
                    
                    # get filename, check if it is .go file
                    s = ss[0]
                    if not s.endswith(".go"):
                        continue

                    # check it is not go-multiaddr-***, should be either "/go-multiaddr" or "/go-multiaddr/"
                    orig = ss[-1]
                    pos = orig.find(lib)
                    if pos != -1:
                        next_pos = pos+len(lib)
                        if next_pos == len(orig):
                            continue
                        elif orig[next_pos] != "\"" and \
                            orig[next_pos] != "/":
                            # not end with " or /
                            continue
                    else:
                        continue

                    # get lib name, add to nexlibs
                    pos = s.find("@v")
                    if pos != -1:
                        sub = s[:pos]
                        pkg = sub.split("/")[-1]
                        if pkg not in next_libs and pkg not in visited_libs:
                            next_libs.append(pkg)
                            if lib not in d:
                                d[lib] = []
                            d[lib].append(pkg)

        libs = next_libs
        visited_libs.extend(libs)
        for key, value in d.iteritems():
            print key, value
        print "****** round end ******\n"

    print "\ndependency dictionary:\n"
    allchange = set()
    for key, value in d.iteritems():
        allchange.add(key)
        for item in value:
            allchange.add(item)
    print "\nall", len(allchange), "libs to change\n"
    print allchange

    print_dep(d)

if __name__ == "__main__":
    # check GOPATH
    if 'GOPATH' not in os.environ:
        print "GOPATH not set"
        os._exit(1)
    run()
