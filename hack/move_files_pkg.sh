#!/bin/bash
# Handle the move of files pkg to monogo pkg
set -ex
ORIGINAL="github.com\/jlewi\/hydros\/pkg\/files"
NEW="github.com\/jlewi\/monogo\/files"
find ./ -name "*.go"  -exec  sed -i ".bak" "s/${ORIGINAL}/${NEW}/g" {} ";"
sed -i ".bak" "s/${ORIGINAL}/${NEW}/g" go.mod
find ./ -name "*.bak" -exec rm {} ";"
