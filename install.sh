#!/usr/bin/env bash

# error codes
# 0 - exited without problems
# 1 - parameters not supported were used or some unexpected error occurred
# 2 - OS not supported by this script
# 3 - installed version of eclone is up to date
# 4 - supported unzip tools are not available

set -e

REPO="ebadenes/eclone"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"
GITHUB_DL="https://github.com/${REPO}/releases/download"

#when adding a tool to the list make sure to also add its corresponding command further in the script
unzip_tools_list=('unzip' '7z' 'busybox')

usage() { echo "Usage: sudo -v ; curl https://raw.githubusercontent.com/${REPO}/master/install.sh | sudo bash [-s beta]" 1>&2; exit 1; }

#check for beta flag
if [ -n "$1" ] && [ "$1" != "beta" ]; then
    usage
fi

if [ -n "$1" ]; then
    install_beta="beta "
fi


#create tmp directory and move to it with macOS compatibility fallback
tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t 'eclone-install.XXXXXXXXXX')
cd "$tmp_dir"


#make sure unzip tool is available and choose one to work with
set +e
for tool in ${unzip_tools_list[*]}; do
    trash=$(hash "$tool" 2>>errors)
    if [ "$?" -eq 0 ]; then
        unzip_tool="$tool"
        break
    fi
done
set -e

# exit if no unzip tools available
if [ -z "$unzip_tool" ]; then
    printf "\nNone of the supported tools for extracting zip archives (${unzip_tools_list[*]}) were found. "
    printf "Please install one of them and try again.\n\n"
    exit 4
fi

# Make sure we don't create a root owned .config/rclone directory #2127
export XDG_CONFIG_HOME=config

#check installed version of eclone to determine if update is necessary
version=$(eclone --version 2>>errors | head -n 1 || true)

# Get latest version from GitHub releases
if [ -z "$install_beta" ]; then
    current_version=$(curl -fsS "$GITHUB_API" | grep '"tag_name"' | cut -d'"' -f4)
    if [ -z "$current_version" ]; then
        printf "\nFailed to get latest version from GitHub. Check your internet connection.\n\n"
        exit 1
    fi
    current_version="eclone ${current_version}"
else
    # For beta, get the latest pre-release
    current_version=$(curl -fsS "https://api.github.com/repos/${REPO}/releases" | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    if [ -z "$current_version" ]; then
        printf "\nFailed to get latest beta version from GitHub.\n\n"
        exit 1
    fi
    current_version="eclone ${current_version}"
fi

if [ "$version" = "$current_version" ]; then
    printf "\nThe latest ${install_beta}version of ${version} is already installed.\n\n"
    exit 3
fi


#detect the platform
OS="$(uname)"
case $OS in
  Linux)
    OS='linux'
    ;;
  FreeBSD)
    OS='freebsd'
    ;;
  NetBSD)
    OS='netbsd'
    ;;
  OpenBSD)
    OS='openbsd'
    ;;
  Darwin)
    OS='osx'
    binTgtDir=/usr/local/bin
    ;;
  SunOS)
    OS='solaris'
    echo 'OS not supported'
    exit 2
    ;;
  *)
    echo 'OS not supported'
    exit 2
    ;;
esac

OS_type="$(uname -m)"
case "$OS_type" in
  x86_64|amd64)
    OS_type='amd64'
    ;;
  i?86|x86)
    OS_type='386'
    ;;
  aarch64|arm64)
    OS_type='arm64'
    ;;
  armv7*)
    OS_type='arm-v7'
    ;;
  armv6*)
    OS_type='arm-v6'
    ;;
  arm*)
    OS_type='arm'
    ;;
  *)
    echo 'OS type not supported'
    exit 2
    ;;
esac


#download and unzip
# Extract version tag from current_version string (e.g., "eclone v1.73.0-mod2.0.2" -> "v1.73.0-mod2.0.2")
version_tag=$(echo "$current_version" | awk '{print $2}')
download_link="${GITHUB_DL}/${version_tag}/eclone-${version_tag}-${OS}-${OS_type}.zip"
eclone_zip="eclone-${version_tag}-${OS}-${OS_type}.zip"

printf "Downloading eclone ${version_tag} for ${OS}/${OS_type}...\n"
curl -OfsSL "$download_link"
if [ $? -ne 0 ]; then
    printf "\nFailed to download ${download_link}\n"
    printf "Please check if this version/platform is available at https://github.com/${REPO}/releases\n\n"
    exit 1
fi

unzip_dir="tmp_unzip_dir_for_eclone"
# there should be an entry in this switch for each element of unzip_tools_list
case "$unzip_tool" in
  'unzip')
    unzip -a "$eclone_zip" -d "$unzip_dir"
    ;;
  '7z')
    7z x "$eclone_zip" "-o$unzip_dir"
    ;;
  'busybox')
    mkdir -p "$unzip_dir"
    busybox unzip "$eclone_zip" -d "$unzip_dir"
    ;;
esac

cd $unzip_dir/*

#mounting eclone to environment

case "$OS" in
  'linux')
    #binary
    cp eclone /usr/bin/eclone.new
    chmod 755 /usr/bin/eclone.new
    chown root:root /usr/bin/eclone.new
    mv /usr/bin/eclone.new /usr/bin/eclone
    ;;
  'freebsd'|'openbsd'|'netbsd')
    #binary
    cp eclone /usr/bin/eclone.new
    chown root:wheel /usr/bin/eclone.new
    mv /usr/bin/eclone.new /usr/bin/eclone
    ;;
  'osx')
    #binary
    mkdir -m 0555 -p ${binTgtDir}
    cp eclone ${binTgtDir}/eclone.new
    mv ${binTgtDir}/eclone.new ${binTgtDir}/eclone
    chmod a=x ${binTgtDir}/eclone
    ;;
  *)
    echo 'OS not supported'
    exit 2
esac

#update version variable post install
version=$(eclone --version 2>>errors | head -n 1)

#cleanup
rm -rf "$tmp_dir"

printf "\n${version} has successfully installed."
printf '\nNow run "eclone config" for setup. Check https://github.com/ebadenes/eclone for more details.\n\n'
exit 0
