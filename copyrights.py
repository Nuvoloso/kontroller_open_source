#! /usr/bin/env python2.7
# -*- coding: UTF-8 -*-
"""
Copyright 2019 Tad Lebeck

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Walks the tree checking files for copyrights.
Only files known to git are checked.
"""
import io
import re
import subprocess
import sys
import time

# regexp of files to skip (swagger.yaml and .gitignore are handled specially)
SKIP = r'(.*\.(config|crt|gotmpl|ini|json|key|md|yaml|yaml\.tmpl)|' \
    '.*/csi_pb/csi.pb.go|.*/csi_pb/csi.proto|go.mod|go.sum|' \
    '.*/Dockerfile.*|.*/mock_.*|.*/README|.*/testdata/namespace|.*/testdata/token)$'


def should_check(file_name):
    """Determines if a file should be checked for a copyright"""

    if file_name in ['api/swagger.yaml', 'deploy/centrald/lib/deploy/mc.yaml.tmpl',
                     'deploy/centrald/lib/deploy/mc-csi.yaml.tmpl']:
        return True
    if file_name == '.gitignore' or file_name == '':
        return False
    if re.match(SKIP, file_name):
        return False
    return True


def get_last_commit_year(file_name):
    """Get year of last commit to the named file"""

    cmd_list = [
        'git', 'log', '-1', '--format=%ad', '--date=format-local:%Y', file_name
    ]
    data = ''
    try:
        data = subprocess.check_output(cmd_list)
    except subprocess.CalledProcessError as exc:
        data = exc.output
        if data:
            print data
            raise
    # if not data:  # new file
    #    return time.localtime().tm_year
    return int(data)


def check(file_name, is_modified):
    """Check the file for a valid copyright near the top of the file.

    Returns 0 if a copyright was found, 1 if not.
    """

    this_year = time.localtime().tm_year
    if file_name != 'api/swagger.yaml':
        pattern = r'.*Copyright Tad Lebeck (\d{4})\b'
        max_line = 2
        if file_name.endswith('.py'):
            max_line = 4
        elif file_name.endswith('.go'):
            max_line = 1
    else:
        pattern = ur'.*Copyright © (\d{4}) Tad Lebeck, All Rights Reserved$'
        max_line = 12
    prog = re.compile(pattern)
    try:
        with io.open(file_name, 'r', encoding='utf-8') as file_obj:
            line_no = 0
            for line in file_obj:
                line_no += 1
                if line_no > max_line:  # arbitrary
                    break
                match = prog.match(line)
                if match:
                    year = int(match.group(1))
                    if is_modified:
                        if year == this_year:
                            return 0
                    else:
                        file_year = get_last_commit_year(file_name)
                        if year == file_year:
                            return 0
                    print '%s: Copyright %d needs to be updated' % (file_name, year)
                    return 1
    except IOError:
        # on jenkins, deleted or renamed files are returned, ignore the error
        return 0
    except Exception:
        print 'Unknown exception in file %s' %(file_name)

    print '%s: no valid Copyright found in the first %d lines' % (file_name, max_line)
    return 1


def get_uncommitted_file_list():
    """Return a list of files that are not committed."""

    cmd_list = [
        'git', 'status', '-s'
    ]
    data = ''
    try:
        data = subprocess.check_output(cmd_list)
    except subprocess.CalledProcessError as exc:
        data = exc.output
        if data:
            print data
            raise

    modified = []
    prog = re.compile('([AMU][ AMU]|[ R]M) ')
    for status in data.split('\n'):
        if prog.match(status):
            if ' -> ' in status:
                parts = status.split(' -> ')
                modified.append(parts[1])
            else:
                modified.append(status[3:])
    return modified


def walk(walk_all):
    """Walk the files in the repo. If walk_all is True, all files are walked.
    Otherwise, only files modified in the current branch are walked.
    """

    verbose = '-v' in sys.argv
    if verbose:
        print 'Checking copyrights'
    cmd_list = ['git', 'diff', '--name-only', 'origin/master...HEAD']
    if walk_all:
        cmd_list = ['git', 'ls-files']
    retcode = 0
    data = ''
    try:
        data = subprocess.check_output(cmd_list)
    except subprocess.CalledProcessError as exc:
        retcode = exc.returncode
        data = exc.output
        if data:
            print data
            return retcode
    modified = get_uncommitted_file_list()
    files = list(set().union(data.strip().split('\n'), modified))
    files.sort()

    errors = 0
    for file_name in files:
        if should_check(file_name):
            if verbose:
                print 'Checking', file_name
            is_modified = True if file_name in modified else False
            errors += check(file_name, is_modified)
        elif file_name and verbose:
            print 'Skipped', file_name
    if errors:
        retcode = 1
    return retcode


if __name__ == '__main__':
    if '-h' in sys.argv or '--help' in sys.argv:
        print 'usage: copyrights.py [-v] [--all]'
        print ''
        print 'Checks copyrights of files modified in the current branch or all files if --all'
        sys.exit(0)
    sys.exit(walk('--all' in sys.argv))
