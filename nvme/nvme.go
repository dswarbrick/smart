// Copyright 2017-18 Daniel Swarbrick. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// DEPRECATED - use github.com/dswarbrick/go-nvme/nvme directly.

package nvme

import (
	"io"

	"github.com/dswarbrick/smart/drivedb"

	"github.com/dswarbrick/go-nvme/nvme"
)

type NVMeDevice struct {
	Name  string
	_nvme nvme.NVMeDevice
}

func NewNVMeDevice(name string) *NVMeDevice {
	return &NVMeDevice{name, *nvme.NewNVMeDevice(name)}
}

func (d *NVMeDevice) Open() (err error) {
	return d._nvme.Open()
}

func (d *NVMeDevice) Close() error {
	return d._nvme.Close()
}

// WIP - need to split out functionality further.
func (d *NVMeDevice) PrintSMART(db *drivedb.DriveDb, w io.Writer) error {
	if _, err := d._nvme.IdentifyController(w); err != nil {
		return err
	}

	if err := d._nvme.IdentifyNamespace(w, 1); err != nil {
		return err
	}

	return d._nvme.PrintSMART(w)
}
