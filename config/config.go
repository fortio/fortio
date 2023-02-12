// Copyright 2023 Fortio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Intermediate adapter between dflag and library config.
// Allowing library to set config default values without
// forcing flags to show up in the callers.
package config

import "fortio.org/dflag"

type Config[t any] interface {
	Set(rawInput string) error
	Get() t
	Usage() string
}

type DefaultValue[t dflag.DynValueTypes] struct {
	value t
	usage string
}

func (d *DefaultValue[t]) Get() t {
	return d.value
}

func (d *DefaultValue[t]) Set(inp string) error {
	v, err := dflag.Parse[t](inp)
	if err != nil {
		return err
	}
	d.value = v
	return nil
}

func (d *DefaultValue[t]) Usage() string {
	return d.usage
}

func New[t dflag.DynValueTypes](v t, info string) Config[t] {
	return &DefaultValue[t]{value: v, usage: info}
}
