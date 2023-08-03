#! /bin/bash
# Update the README.md file with the latest release number
# and the flags
# Usage: ./bumpRelease.sh <release number> without the v
# (use something else to test and/or git checkout -- README.md after each test)
set -e
FILENAME="README.md"
RELEASE=$1
CURRENT=$(head -1 $FILENAME | awk '/<!-- ([^- ]+) -->$/ { print $2}')
if [ -z "$CURRENT" ]; then
    echo "Cannot find current release in $FILENAME"
    exit 1
fi
if [ -z "$RELEASE" ]; then
    echo "Usage: $0 <release number>, eg 1.53.0 without the v, to change from current $CURRENT"
    exit 1
fi
if [ "$CURRENT" = "$RELEASE" ]; then
    if [ "$2" = "-f" ]; then
        echo "Forcing regen of same $CURRENT"
    else
        echo "Current release $CURRENT is the same as the requested, nothing to do, add -f to force"
        exit 0
    fi
fi
echo "Changing $FILENAME from $CURRENT to release $RELEASE and updating usage section"
./release/updateFlags.sh | sed -e "s/Φορτίο dev/Φορτίο $RELEASE/" > /tmp/fortio_flags.txt
cp README.md README.md.bak
SEARCH=${CURRENT//\./\\.} # escape dots so we don't go and replace 12534 in a timestamp when looking for 1.5.4
sed -e "s/$SEARCH/$RELEASE/g" README.md.bak | \
   awk '/USAGE_START/ {print $0; skip=1; system("cat /tmp/fortio_flags.txt")} /USAGE_END/ {skip=0} {if (!skip) print $0}' \
   > README.md
echo "DONE. Check the diff:"
git diff
