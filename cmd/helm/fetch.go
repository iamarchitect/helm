/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/helm/cmd/helm/downloader"
	"k8s.io/helm/cmd/helm/helmpath"
	"k8s.io/helm/pkg/chartutil"
)

const fetchDesc = `
Retrieve a package from a package repository, and download it locally.

This is useful for fetching packages to inspect, modify, or repackage. It can
also be used to perform cryptographic verification of a chart without installing
the chart.

There are options for unpacking the chart after download. This will create a
directory for the chart and uncomparess into that directory.

If the --verify flag is specified, the requested chart MUST have a provenance
file, and MUST pass the verification process. Failure in any part of this will
result in an error, and the chart will not be saved locally.
`

type fetchCmd struct {
	untar    bool
	untardir string
	chartRef string
	destdir  string
	version  string

	verify      bool
	verifyLater bool
	keyring     string

	out io.Writer
}

func newFetchCmd(out io.Writer) *cobra.Command {
	fch := &fetchCmd{out: out}

	cmd := &cobra.Command{
		Use:   "fetch [flags] [chart URL | repo/chartname] [...]",
		Short: "download a chart from a repository and (optionally) unpack it in local directory",
		Long:  fetchDesc,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("This command needs at least one argument, url or repo/name of the chart.")
			}
			for i := 0; i < len(args); i++ {
				fch.chartRef = args[i]
				if err := fch.run(); err != nil {
					return err
				}
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&fch.untar, "untar", false, "if set to true, will untar the chart after downloading it")
	f.StringVar(&fch.untardir, "untardir", ".", "if untar is specified, this flag specifies the name of the directory into which the chart is expanded")
	f.BoolVar(&fch.verify, "verify", false, "verify the package against its signature")
	f.BoolVar(&fch.verifyLater, "prov", false, "fetch the provenance file, but don't perform verification")
	f.StringVar(&fch.version, "version", "", "specific version of a chart. Without this, the latest version is fetched")
	f.StringVar(&fch.keyring, "keyring", defaultKeyring(), "keyring containing public keys")
	f.StringVarP(&fch.destdir, "destination", "d", ".", "location to write the chart. If this and tardir are specified, tardir is appended to this")

	return cmd
}

func (f *fetchCmd) run() error {
	pname := f.chartRef
	c := downloader.ChartDownloader{
		HelmHome: helmpath.Home(homePath()),
		Out:      f.out,
		Keyring:  f.keyring,
		Verify:   downloader.VerifyNever,
	}

	if f.verify {
		c.Verify = downloader.VerifyAlways
	} else if f.verifyLater {
		c.Verify = downloader.VerifyLater
	}

	// If untar is set, we fetch to a tempdir, then untar and copy after
	// verification.
	dest := f.destdir
	if f.untar {
		var err error
		dest, err = ioutil.TempDir("", "helm-")
		if err != nil {
			return fmt.Errorf("Failed to untar: %s", err)
		}
		defer os.RemoveAll(dest)
	}

	saved, v, err := c.DownloadTo(pname, f.version, dest)
	if err != nil {
		return err
	}

	if f.verify {
		fmt.Fprintf(f.out, "Verification: %v", v)
	}

	// After verification, untar the chart into the requested directory.
	if f.untar {
		ud := f.untardir
		if !filepath.IsAbs(ud) {
			ud = filepath.Join(f.destdir, ud)
		}
		if fi, err := os.Stat(ud); err != nil {
			if err := os.MkdirAll(ud, 0755); err != nil {
				return fmt.Errorf("Failed to untar (mkdir): %s", err)
			}

		} else if !fi.IsDir() {
			return fmt.Errorf("Failed to untar: %s is not a directory", ud)
		}

		return chartutil.ExpandFile(ud, saved)
	}
	return nil
}

// defaultKeyring returns the expanded path to the default keyring.
func defaultKeyring() string {
	return os.ExpandEnv("$HOME/.gnupg/pubring.gpg")
}
