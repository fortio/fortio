#! /bin/bash
# Extract fortio's help and rewrap it to 80 cols
# fmt doesn't touch lines starting with . so we change the " -" to dot and back to keep
# the option lines
fortio help | sed -e 's/^  -/./' | fmt -80 | sed -e 's/^\./  -/'
