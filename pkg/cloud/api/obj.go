/*
Copyright 2023 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"fmt"
	"reflect"
)

// ConversionContext gives which version => version the error occurred on.
type ConversionContext int

const (
	GAToAlphaConversion ConversionContext = iota
	GAToBetaConversion
	AlphaToGAConversion
	AlphaToBetaConversion
	BetaToGAConversion
	BetaToAlphaConversion
	conversionContextCount // Sentinel value used to size arrays.
)

// ConversionError is returned from To*() methods. Inspect this error to get
// more details on what did not convert.
type ConversionError struct {
	// MissingFields is a list of field values that were set but did not
	// translate to the version requested.
	MissingFields []MissingField
}

func (e *ConversionError) hasErr() bool {
	return len(e.MissingFields) > 0
}

// Error implements error.
func (e *ConversionError) Error() string {
	return fmt.Sprintf("ConversionError: missing fields %v", e.MissingFields)
}

// MissingField describes a field that was lost when converting between API
// versions due to the field not being present in struct.
type MissingField struct {
	// Context gives the version to => from.
	Context ConversionContext
	// Path of the field that is missing.
	Path Path
	// Value of the source field.
	Value any
}

type conversionErrors struct {
	missingFields []missingFieldOnCopy
}

// VersionedObject wraps the standard GA, Alpha, Beta versions of a GCP
// resource. By accessing the object using Access(), AccessAlpha().
// AccessBeta(), VersionedObject will ensure that common fields between the
// versions of the object are in sync.
type VersionedObject[GA any, Alpha any, Beta any] struct {
	copierOptions []copierOption

	ga    GA
	alpha Alpha
	beta  Beta

	errors [conversionContextCount]conversionErrors
}

// CheckSchema should be called in init() to ensure that the resource being
// wrapped by VersionedObject meets the assumptions we are making for this the
// transformations to work.
func (u *VersionedObject[GA, Alpha, Beta]) CheckSchema() error {
	err := checkSchema(reflect.TypeOf(u.ga))
	if err != nil {
		return err
	}
	err = checkSchema(reflect.TypeOf(u.alpha))
	if err != nil {
		return err
	}
	err = checkSchema(reflect.TypeOf(u.beta))
	if err != nil {
		return err
	}
	return nil
}

// Access the mutable object.
func (u *VersionedObject[GA, Alpha, Beta]) Access(f func(x *GA)) error {
	f(&u.ga)

	src := reflect.ValueOf(&u.ga)

	c := newCopier(u.copierOptions...)
	err := c.do(reflect.ValueOf(&u.alpha), src)
	if err != nil {
		return err
	}
	u.errors[GAToAlphaConversion].missingFields = c.missing

	c = newCopier(u.copierOptions...)
	err = c.do(reflect.ValueOf(&u.beta), src)
	if err != nil {
		return err
	}
	u.errors[GAToBetaConversion].missingFields = c.missing

	return nil
}

// AccessAlpha object.
func (u *VersionedObject[GA, Alpha, Beta]) AccessAlpha(f func(x *Alpha)) error {
	f(&u.alpha)
	src := reflect.ValueOf(&u.alpha)

	c := newCopier(u.copierOptions...)
	err := c.do(reflect.ValueOf(&u.ga), src)
	if err != nil {
		return err
	}
	u.errors[AlphaToGAConversion].missingFields = c.missing

	c = newCopier(u.copierOptions...)
	err = c.do(reflect.ValueOf(&u.beta), src)
	if err != nil {
		return err
	}
	u.errors[AlphaToBetaConversion].missingFields = c.missing

	return nil
}

// AccessBeta object.
func (u *VersionedObject[GA, Alpha, Beta]) AccessBeta(f func(x *Beta)) error {
	f(&u.beta)
	src := reflect.ValueOf(&u.beta)

	c := newCopier(u.copierOptions...)
	err := c.do(reflect.ValueOf(&u.ga), src)
	if err != nil {
		return err
	}
	u.errors[BetaToGAConversion].missingFields = c.missing

	c = newCopier(u.copierOptions...)
	err = c.do(reflect.ValueOf(&u.alpha), src)
	if err != nil {
		return err
	}
	u.errors[BetaToAlphaConversion].missingFields = c.missing

	return nil
}

// ToGA returns the GA version of this object. Use error.As ConversionError to
// get the specific details.
func (u *VersionedObject[GA, Alpha, Beta]) ToGA() (*GA, error) {
	var errs ConversionError
	for _, cc := range []ConversionContext{AlphaToGAConversion, BetaToGAConversion} {
		for _, mf := range u.errors[cc].missingFields {
			errs.MissingFields = append(errs.MissingFields, MissingField{
				Context: cc,
				Path:    mf.Path,
				Value:   mf.Value,
			})
		}
	}
	if errs.hasErr() {
		return &u.ga, &errs
	}
	return &u.ga, nil
}

// ToAlpha returns the Alpha version of this object. Use error.As
// ConversionError to get the specific details.
func (u *VersionedObject[GA, Alpha, Beta]) ToAlpha() (*Alpha, error) {
	var errs ConversionError
	for _, cc := range []ConversionContext{GAToAlphaConversion, BetaToAlphaConversion} {
		for _, mf := range u.errors[cc].missingFields {
			errs.MissingFields = append(errs.MissingFields, MissingField{
				Context: cc,
				Path:    mf.Path,
				Value:   mf.Value,
			})
		}
	}
	if errs.hasErr() {
		return &u.alpha, &errs
	}
	return &u.alpha, nil
}

// ToBeta returns the Beta version of this object. Use error.As ConversionError
// to get the specific details.
func (u *VersionedObject[GA, Alpha, Beta]) ToBeta() (*Beta, error) {
	var errs ConversionError
	for _, cc := range []ConversionContext{GAToBetaConversion, AlphaToBetaConversion} {
		for _, mf := range u.errors[cc].missingFields {
			errs.MissingFields = append(errs.MissingFields, MissingField{
				Context: cc,
				Path:    mf.Path,
				Value:   mf.Value,
			})
		}
	}
	if errs.hasErr() {
		return &u.beta, &errs
	}
	return &u.beta, nil
}

// Set the value to src.
func (u *VersionedObject[GA, Alpha, Beta]) Set(src *GA) error {
	var err error
	u.Access(func(dest *GA) {
		c := newCopier(u.copierOptions...)
		err = c.do(reflect.ValueOf(dest), reflect.ValueOf(src))
	})
	return err
}

// SetAlpha the value to src.
func (u *VersionedObject[GA, Alpha, Beta]) SetAlpha(src *Alpha) error {
	var err error
	u.AccessAlpha(func(dest *Alpha) {
		c := newCopier(u.copierOptions...)
		err = c.do(reflect.ValueOf(dest), reflect.ValueOf(src))
	})
	return err
}

// SetBeta the value to src.
func (u *VersionedObject[GA, Alpha, Beta]) SetBeta(src *Beta) error {
	var err error
	u.AccessBeta(func(dest *Beta) {
		c := newCopier(u.copierOptions...)
		err = c.do(reflect.ValueOf(dest), reflect.ValueOf(src))
	})
	return err
}
