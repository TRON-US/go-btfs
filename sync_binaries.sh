#!/usr/bin/env bash

# Make sure S3Location is set
if [ -z "$S3Location" ]
then
    echo "\$S3Location must be set. Please set the S3Location"
    exit
fi

declare -a OS_VALUE=("darwin" "linux")
declare -a ARCH_VALUE=("386" "amd64" "arm" "arm64")

cd ../btfs-binary-releases

# Delete existing files
echo "=== Deleting existing files ==="
rm -rf ./darwin/386/*
rm -rf ./darwin/amd64/*
rm -rf ./linux/386/*
rm -rf ./linux/amd64/*
rm -rf ./linux/arm/*
rm -rf ./linux/arm64/*
rm -rf ./windows/386/*
rm -rf ./windows/amd64/*
echo "=== Completed deleting existing files ==="

# Get the version for fs-repo-migrations
curl -fsSL -O https://github.com/TRON-US/btfs-distributions/raw/master/fs-repo-migrations/versions
if [ $? -eq 0 ]
then
    VERSION=$(cat versions | sort -V | tail -n 1)
    rm -f versions
else
    echo "Download of fs-repo-migrations version file failed, confirm your internet connection to GitHub is working and rerun this script."
    exit 1
fi

# Download files for macOS and Linux
for OS in ${OS_VALUE[@]}; do
    for ARCH in ${ARCH_VALUE[@]}; do

        if [[ ${OS} = "darwin" && ( ${ARCH} = "arm64" || ${ARCH} = "arm" ) ]]; then continue; fi

        echo "=== Performing dload for "$OS" "$ARCH" ==="
        cd "$OS"/"$ARCH"
        wget -q distributions.btfs.io/"$S3Location"/"$OS"/"$ARCH"/btfs-"$OS"-"$ARCH".tar.gz
        gunzip btfs-"$OS"-"$ARCH".tar.gz
        tar -xf btfs-"$OS"-"$ARCH".tar
        wget -q distributions.btfs.io/"$S3Location"/"$OS"/"$ARCH"/update-"$OS"-"$ARCH".tar.gz
        tar -xzf update-"$OS"-"$ARCH".tar.gz
        rm update-"$OS"-"$ARCH".tar.gz
        URL="https://github.com/TRON-US/btfs-distributions/raw/master/fs-repo-migrations/${VERSION}/fs-repo-migrations_${VERSION}_${OS}-${ARCH}.tar.gz"
        wget -q "$URL"
        tar -xzf fs-repo-migrations_${VERSION}_${OS}-${ARCH}.tar.gz
        mv fs-repo-migrations/fs-repo-migrations fs-repo-migrations-${OS}-${ARCH}
        rm -f fs-repo-migrations_${VERSION}_${OS}-${ARCH}.tar.gz
        rm -rf fs-repo-migrations

        cd ../..
    done
done

# Download files for Windows
OS="windows"
for ARCH in ${ARCH_VALUE[@]}; do

    if [[ ${ARCH} = "arm64" || ${ARCH} = "arm" ]]; then continue; fi

    echo "=== Performing dload for windows "$ARCH" ==="
    cd windows/"$ARCH"
    wget -q distributions.btfs.io/"$S3Location"/windows/"$ARCH"/btfs-windows-"$ARCH".zip
    unzip -q btfs-windows-"$ARCH".zip
    wget -q distributions.btfs.io/"$S3Location"/windows/"$ARCH"/update-windows-"$ARCH".zip
    unzip -q update-windows-"$ARCH".zip
    rm update-windows-"$ARCH".zip
    URL="https://github.com/TRON-US/btfs-distributions/raw/master/fs-repo-migrations/${VERSION}/fs-repo-migrations_${VERSION}_${OS}-${ARCH}.zip"
    wget -q "$URL"
    unzip -q fs-repo-migrations_${VERSION}_${OS}-${ARCH}.zip
    mv fs-repo-migrations/fs-repo-migrations.exe fs-repo-migrations-${OS}-${ARCH}.exe
    rm -f fs-repo-migrations_${VERSION}_${OS}-${ARCH}.zip
    rm -rf fs-repo-migrations
    
    cd ../..
done
