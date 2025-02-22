// Copyright © 2023 Cisco Systems, Inc. and its affiliates.
// All rights reserved.
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

package gorm

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/openclarity/vmclarity/pkg/shared/utils"
)

type patchObjectTestNestedObject struct {
	NestedString string `json:"nestedString,omitempty"`
}

type patchObjectTestObject struct {
	TestString         *string                        `json:"testString,omitempty"`
	TestInt            *int                           `json:"testInt,omitempty"`
	TestBool           *bool                          `json:"testBool,omitempty"`
	TestNestedObject   *patchObjectTestNestedObject   `json:"testNestedObject,omitempty"`
	TestArrayPrimitive *[]string                      `json:"testArrayPrimitive,omitempty"`
	TestArrayObject    *[]patchObjectTestNestedObject `json:"testArrayObject,omitempty"`
}

func Test_patchObject(t *testing.T) {
	var nilArray []string

	tests := []struct {
		name     string
		existing patchObjectTestObject
		patch    patchObjectTestObject
		want     patchObjectTestObject
		wantErr  bool
	}{
		{
			name:     "patch over unset",
			existing: patchObjectTestObject{},
			patch: patchObjectTestObject{
				TestString: utils.PointerTo("foo"),
				TestInt:    utils.PointerTo(1),
				TestBool:   utils.PointerTo(false),
				TestNestedObject: &patchObjectTestNestedObject{
					NestedString: "nestedfoo",
				},
				TestArrayPrimitive: utils.PointerTo([]string{"arrayfoo"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedfoo",
					},
				}),
			},
			want: patchObjectTestObject{
				TestString: utils.PointerTo("foo"),
				TestInt:    utils.PointerTo(1),
				TestBool:   utils.PointerTo(false),
				TestNestedObject: &patchObjectTestNestedObject{
					NestedString: "nestedfoo",
				},
				TestArrayPrimitive: utils.PointerTo([]string{"arrayfoo"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedfoo",
					},
				}),
			},
			wantErr: false,
		},
		{
			name: "patch over already set",
			existing: patchObjectTestObject{
				TestString: utils.PointerTo("foo"),
				TestInt:    utils.PointerTo(1),
				TestBool:   utils.PointerTo(false),
				TestNestedObject: &patchObjectTestNestedObject{
					NestedString: "nestedfoo",
				},
				TestArrayPrimitive: utils.PointerTo([]string{"arrayfoo"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedfoo",
					},
				}),
			},
			patch: patchObjectTestObject{
				TestString: utils.PointerTo("bar"),
				TestInt:    utils.PointerTo(2),
				TestBool:   utils.PointerTo(true),
				TestNestedObject: &patchObjectTestNestedObject{
					NestedString: "nestedbar",
				},
				TestArrayPrimitive: utils.PointerTo([]string{"arraybar"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedbar",
					},
				}),
			},
			want: patchObjectTestObject{
				TestString: utils.PointerTo("bar"),
				TestInt:    utils.PointerTo(2),
				TestBool:   utils.PointerTo(true),
				TestNestedObject: &patchObjectTestNestedObject{
					NestedString: "nestedbar",
				},
				TestArrayPrimitive: utils.PointerTo([]string{"arraybar"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedbar",
					},
				}),
			},
			wantErr: false,
		},
		{
			name: "patch Int, Bool, ArrayPrimitive and ArrayObject, String and NestedObject already set",
			existing: patchObjectTestObject{
				TestString: utils.PointerTo("foo"),
				TestInt:    utils.PointerTo(1),
				TestBool:   utils.PointerTo(false),
				TestNestedObject: &patchObjectTestNestedObject{
					NestedString: "nestedfoo",
				},
				TestArrayPrimitive: utils.PointerTo([]string{"arrayfoo"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedfoo",
					},
				}),
			},
			patch: patchObjectTestObject{
				TestInt:            utils.PointerTo(2),
				TestBool:           utils.PointerTo(true),
				TestArrayPrimitive: utils.PointerTo([]string{"arraybar"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedbar",
					},
				}),
			},
			want: patchObjectTestObject{
				TestString: utils.PointerTo("foo"),
				TestInt:    utils.PointerTo(2),
				TestBool:   utils.PointerTo(true),
				TestNestedObject: &patchObjectTestNestedObject{
					NestedString: "nestedfoo",
				},
				TestArrayPrimitive: utils.PointerTo([]string{"arraybar"}),
				TestArrayObject: utils.PointerTo([]patchObjectTestNestedObject{
					{
						NestedString: "arraynestedbar",
					},
				}),
			},
			wantErr: false,
		},
		{
			name: "patch null to unset a field",
			existing: patchObjectTestObject{
				TestArrayPrimitive: utils.PointerTo([]string{"arraybar"}),
			},
			patch: patchObjectTestObject{
				TestArrayPrimitive: &nilArray,
			},
			want:    patchObjectTestObject{},
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			existing, err := json.Marshal(test.existing)
			if err != nil {
				t.Fatalf("failed to marshal existing object: %v", err)
			}

			updatedBytes, err := patchObject(existing, test.patch)
			if err != nil {
				if !test.wantErr {
					t.Fatalf("unexpected error: %v", err)
				}
				// Expected this error so return successful
				return
			}

			var updated patchObjectTestObject
			err = json.Unmarshal(updatedBytes, &updated)
			if err != nil {
				t.Fatalf("failed to unmarshal updated bytes: %v", err)
			}

			if diff := cmp.Diff(test.want, updated); diff != "" {
				t.Fatalf("patchObject mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
