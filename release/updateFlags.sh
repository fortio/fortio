#! /bin/bash
# Copyright 2017 Fortio Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Extract fortio's help and rewrap it to 90 cols (as MD doesn't wrap code blocks)
# Also remove the /var/folders/fq/gng4z4915mb73r9js_422h4c0000gn/T/go-build179128464/b001/exe/ noise
# The 2>&1 isn't because the help goes to stderr (it doesn't) but to make the cli/log package
# detect we're not on the console and thus not use color mode.
go run . help 2>&1 | expand | sed -e 's!/.*/fortio !fortio !' | fold -s -w 90 | sed -e "s/ $//" -e "s/</\&lt;/"
