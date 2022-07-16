#!/bin/sh
#
# Create release tarballs/zip-files
#
name=algernon
version=$(grep -i version main.go | head -1 | cut -d' ' -f4 | cut -d'"' -f1)
echo "Version $version"

export GOBUILD=( go build -mod=vendor -trimpath -ldflags "-w -s" -a -o )
export CGO_ENABLED=0

echo 'Compiling...'

export GOOS=windows
GOARCH=amd64 "${GOBUILD[@]}" $name.exe

# Compress with zip for Windows
# Compress the Windows release
echo "Compressing $name-$version-windows.zip"
mkdir "$name-$version-windows_x86_64_static"
cp $name.1 LICENSE $name.exe "$name-$version-windows_x86_64_static/"
zip -q -r "$name-$version-windows_x86_64_static.zip" "$name-$version-windows_x86_64_static/"
rm -r "$name-$version-windows_x86_64_static"
rm $name.exe

mkdir -p release
mv -v $name-$version* release
