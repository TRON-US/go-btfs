#!/bin/bash

echo "Automatic update script begin!"

version=`cat version.go | tail -n +7 | head -n +1 | awk '{print $4}'`

echo "==========>Get btfs version [${version}]"

os=( darwin linux windows )
arch=( amd64 386 arm64 arm )

for goos in ${os[@]}
do
    for goarch in ${arch[@]}
    do
        if [[ ( ${goos} = "windows" || ${goos} = "darwin" ) && ( ${goarch} = "arm64" || ${goarch} = "arm" ) ]]; then continue; fi
        echo "=============>OS: [${goos}] ARCH: [${goarch}] automatic compiler begin."
        ext=""
        if [[ ${goos} = "windows" ]]; then
            ext=".exe"
        fi
        rm -f ../btfs-binary-releases/${goos}/${goarch}/*
        GOOS=${goos} GOARCH=${goarch} make build
        GOOS=${goos} GOARCH=${goarch} go build -o update-${goos}-${goarch}${ext} autoupdate/main.go
        md5=`openssl md5 ./cmd/btfs/btfs | awk '{print $2}'`
        mv ./cmd/btfs/btfs ../btfs-binary-releases/${goos}/${goarch}/btfs-${goos}-${goarch}${ext}
        mv update-${goos}-${goarch}${ext} ../btfs-binary-releases/${goos}/${goarch}/update-${goos}-${goarch}${ext}
        echo -e "version: ${version: 1: ${#version}-2}\nmd5: ${md5}\nautoupdateFlg: true\nsleepTimeSeconds: 86400\nbeginNumber: 0\nendNumber: 100" > ../btfs-binary-releases/${goos}/${goarch}/config_${goos}_${goarch}.yaml
        cd ../btfs-binary-releases/${goos}/${goarch}
        tar -cvf btfs-${goos}-${goarch}.tar config_${goos}_${goarch}.yaml btfs-${goos}-${goarch}${ext}
        cd -
        echo "=============>OS: [${goos}] ARCH: [${goarch}] automatic compiler success."
    done
done

echo "Automatic update script success!"
