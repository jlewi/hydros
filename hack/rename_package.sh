#!/bin/bash
set -ex
ORIGINAL="github.com\/PrimerAI\/hydros-public"
NEW="github.com\/jlewi\/hydros"
find ./ -name "*.go"  -exec  sed -i ".bak" "s/${ORIGINAL}/${NEW}/g" {} ";"
sed -i ".bak" "s/${ORIGINAL}/${NEW}/g" go.mod
find ./ -name "*.bak" -exec rm {} ";"
