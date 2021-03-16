/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2020 Red Hat, Inc.
 */

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jaypipes/ghw/pkg/snapshot"
	"github.com/spf13/cobra"
)

type snapProcOptions struct {
	cloneDirPath string
	output       string
}

func NewSnapProcCommand(knitOpts *KnitOptions) *cobra.Command {
	opts := &snapProcOptions{}
	snapProc := &cobra.Command{
		Use:   "snapproc",
		Short: "create snapshot of running processes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return makeProcSnapshot(cmd, knitOpts, opts, args)
		},
		Args: cobra.NoArgs,
	}
	snapProc.Flags().StringVarP(&opts.cloneDirPath, "clone-path", "c", "", "proc clone path. If not given, generate random name.")
	snapProc.Flags().StringVarP(&opts.output, "output", "o", "snapproc.tgz", "proc snapshot. Use \"-\" to send on stdout.")
	return snapProc
}

func makeProcSnapshot(cmd *cobra.Command, knitOpts *KnitOptions, opts *snapProcOptions, args []string) error {
	var scratchDir string
	var err error
	if opts.cloneDirPath != "" {
		// if user supplies the path, then the user owns the path. So we intentionally don't remove it once done.
		scratchDir = opts.cloneDirPath
	} else {
		scratchDir, err = ioutil.TempDir("", "kni-snapproc")
		if err != nil {
			return err
		}
		defer os.RemoveAll(scratchDir)
	}

	if err = cloneProcInto(knitOpts.ProcFSRoot, scratchDir); err != nil {
		return err
	}

	return snapshot.PackFrom(opts.output, scratchDir)
}

// the proc layout we care about is more volatile than the ghw/snapshot package expects to deal with. So we need extra care.
func cloneProcInto(procfsRoot, scratchDir string) error {
	baseDir := filepath.Join(scratchDir, "proc")

	var err error
	if err = os.MkdirAll(baseDir, os.ModePerm); err != nil {
		return err
	}

	pidEntries, err := ioutil.ReadDir(procfsRoot)
	if err != nil {
		return err
	}

	for _, pidEntry := range pidEntries {
		if !pidEntry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(pidEntry.Name())
		if err != nil {
			// doesn't look like a pid
			continue
		}

		err = cloneProcEntry(pid, procfsRoot, scratchDir)
		if err != nil {
			// TODO log
			continue
		}
	}
	return nil
}

func cloneProcEntry(pid int, procfsRoot, scratchDir string) error {
	entryDir := filepath.Join(procfsRoot, fmt.Sprintf("%d", pid))
	var err error
	// keep the file open to (kinda-)lock the entry
	handle, err := os.Open(filepath.Join(entryDir, "cmdline"))
	if err != nil {
		return err
	}
	defer handle.Close()

	if err = cloneRunnableData(scratchDir, entryDir, "cmdline", "status"); err != nil {
		return err
	}

	taskRoot := filepath.Join(entryDir, "task")
	tidEntries, err := ioutil.ReadDir(taskRoot)
	if err != nil {
		return err
	}

	for _, tidEntry := range tidEntries {
		if !tidEntry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(tidEntry.Name()); err != nil {
			// doesn't look like a tid
			continue
		}

		err = cloneRunnableData(scratchDir, filepath.Join(taskRoot, tidEntry.Name()), "status")
		if err != nil {
			// TODO log
			continue
		}
	}

	return nil
}

func cloneRunnableData(scratchDir, baseDir string, items ...string) error {
	for _, item := range items {
		if err := os.MkdirAll(filepath.Join(scratchDir, baseDir), os.ModePerm); err != nil {
			return err
		}

		path := filepath.Join(baseDir, item)
		if err := copyFile(path, filepath.Join(scratchDir, path)); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(path, targetPath string) error {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(targetPath, buf, os.ModePerm)
}
