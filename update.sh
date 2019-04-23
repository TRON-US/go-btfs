#!/bin/bash

sleep 5s

DefaultProjectPath=/usr/local/bin/
DefaultDownloadPath=/tmp/

while [[ -n "$1" ]] ;do
    case "$1" in
        -p)
            DefaultProjectPath=$2
            shift 2
            ;;
        -d)
            DefaultDownloadPath=$2
            shift 2
            ;;
        *)
            exit 1
            ;;
    esac
done

rm ${DefaultProjectPath}version.txt
mv ${DefaultDownloadPath}version-latest.txt ${DefaultProjectPath}version.txt
rm ${DefaultDownloadPath}btfs.bk
mv ${DefaultProjectPath}btfs ${DefaultDownloadPath}btfs.bk
mv ${DefaultDownloadPath}btfs-latest ${DefaultProjectPath}btfs
chmod 775 ${DefaultProjectPath}btfs
nohup ${DefaultProjectPath}btfs daemon </dev/null > /dev/null 2>&1 &
