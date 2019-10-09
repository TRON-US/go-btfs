!#bin/bash
#this script downloads the necessary files from distributions.btfs.io into the btfs-binary-releases repo

#make sure the S3Location is set
if [ -z "$S3Location" ]
then
    echo "\$S3Location must be set. Please set the S3Location"
    exit
fi

declare -a OS_VALUE=("darwin" "linux")
declare -a ARCH_VALUE=("386" "amd64")

cd ../btfs-binary-releases

#delete existing files
echo "=== Deleting old files ==="
rm -r ./darwin/386/*
rm -r ./darwin/amd64/*
rm -r ./linux/386/*
rm -r ./linux/amd64/*
rm -r ./windows/386/*
rm -r ./windows/amd64/*
echo "=== Completed deleting old files ==="

#download files for darwin and linux
for OS in ${OS_VALUE[@]}; do
    for ARCH in ${ARCH_VALUE[@]}; do
        echo "=== Performing dload for "$OS" "$ARCH" ==="
        cd "$OS"/"$ARCH"
        pwd
        wget -q distributions.btfs.io/"$S3Location"/"$OS"/"$ARCH"/btfs-"$OS"-"$ARCH".tar.gz 
        gunzip btfs-"$OS"-"$ARCH".tar.gz 
        tar -xf btfs-"$OS"-"$ARCH".tar
        wget -q distributions.btfs.io/"$S3Location"/"$OS"/"$ARCH"/config_"$OS"_"$ARCH".yaml 
        wget -q distributions.btfs.io/"$S3Location"/"$OS"/"$ARCH"/update-"$OS"-"$ARCH".tar.gz
        tar -xf update-"$OS"-"$ARCH".tar.gz
        rm update-"$OS"-"$ARCH".tar.gz

        cd ../..
    done
done

#download files for windows
for ARCH in ${ARCH_VALUE[@]}; do
    echo "=== Performing dload for windows "$ARCH" ==="
    cd windows/"$ARCH"
    pwd
    wget -q distributions.btfs.io/"$S3Location"/windows/"$ARCH"/btfs-windows-"$ARCH".exe.zip
    unzip -q btfs-windows-"$ARCH".exe.zip
    wget -q distributions.btfs.io/"$S3Location"/windows/"$ARCH"/config_windows_"$ARCH".yaml
    wget -q distributions.btfs.io/"$S3Location"/windows/"$ARCH"/update-windows-"$ARCH".exe.zip
    unzip -q update-windows-"$ARCH".exe.zip
    rm update-windows-"$ARCH".exe.zip

    cd ../..
done
