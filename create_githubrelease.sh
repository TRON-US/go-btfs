#!/usr/bin/env bash

#Create a GitHub release and upload release assests

#Go to GitHub webpage -> Settings (at the top right corner) -> Developer setting -> Personal access tokens to create a new token
#Give this new token only the scope (permission): repo (Full control of private repositories)
#Better to get this from an environmental variable
GITHUB_ACCESS_TOKEN=""

REPO_OWNER="TRON-US"
REPO_NAME="go-btfs"
TAG_NAME="" #Release title is set to the same as Tag name
BRANCH="release" #Which branch or commit to create tag from
MESSAGE="" #Release discription
DRAFT="true" #Change to false if manual review on GitHub webpage isn't needed; or use argument: -d false
PRERELEASE="false"

#Get args
while getopts t:b:m:d:p: option
do
	case "${option}"
		in
		t) TAG_NAME="$OPTARG";;
		b) BRANCH="$OPTARG";;
		m) MESSAGE="$OPTARG";;
		d) DRAFT="$OPTARG";;
		p) PRERELEASE="$OPTARG";;
	esac
done

if [ -z "$GITHUB_ACCESS_TOKEN" ] ; then
	echo "GitHub access token (GITHUB_ACCESS_TOKEN) is required, create one on GitHub then add it into this script"
	exit 1
fi

#Tag name is required parameter.
if [ -z "$TAG_NAME" ] ; then
	echo "Usage: create_githubrelease.sh -t <tag_name> [-b <branch>] [-m <release discription>] [-d <true|false>] [-p <true|false>]"
	exit 1
fi

#Set default message
if [ -z "$MESSAGE" ]; then
	MESSAGE=$(printf "Release of version %s" $TAG_NAME)
fi


API_JSON=$(printf '{"tag_name": "%s","target_commitish": "%s","name": "%s","body": "%s","draft": %s,"prerelease": %s}' "$TAG_NAME" "$BRANCH" "$TAG_NAME" "$MESSAGE" "$DRAFT" "$PRERELEASE" )
API_RESPONSE_STATUS=$(curl --data "$API_JSON" -s -i https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases?access_token=$GITHUB_ACCESS_TOKEN)
echo "$API_RESPONSE_STATUS"


#Upload assets
RELEASE_ID=$(echo "$API_RESPONSE_STATUS" | grep -m 1 '"id":' | grep -E -o '[0-9]+')
for file in $(find ../btfs-binary-releases/{darwin,linux,windows}/{386,amd64,arm,arm64} -type f 2>/dev/null) #Change the location for btfs-binary-releases used by sync_binaries.sh accordingly
do
	curl -H "Authorization: token $GITHUB_ACCESS_TOKEN" \
		-H "Content-Type: $(file -b --mime-type $file)" \
		--data-binary @$file \
		"https://uploads.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/$RELEASE_ID/assets?name=$(basename $file)"
done

