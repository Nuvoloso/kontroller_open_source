#!/bin/bash -ex
# Copyright 2019 Tad Lebeck
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
# Build script used by jenkins
# Starting with go1.13, the workspace no longer needs to be below the GOPATH

export PATH=$GOPATH/bin:$HOME/go/bin:$PATH

export BUILD_JOB="$JOB_NAME:$BUILD_NUMBER"
BUILD_RESULT='not run'
TEST_RESULT='not run'
LINT_RESULT='not run'
VET_RESULT='not run'
function summary1 {
  lint_value="value=\"$LINT_RESULT\""
  if [ -s "golint.txt" ]; then
    lint_value="value=\"FAILED\" href=\"artifact/golint.txt\""
  fi
  vet_value="value=\"$VET_RESULT\""
  if [ "$VET_RESULT" = FAILED ]; then
    vet_value="value=\"FAILED\" href=\"artifact/govet.txt\""
  fi
  cat > JenkinsResults01.xml << EOF
<section name="Summary Results" fontcolor="">
  <field name="Build" value="$BUILD_RESULT" />
  <field name="Unit Tests" value="$TEST_RESULT" />
  <field name="Lint" $lint_value />
  <field name="Vet" $vet_value />
</section>
EOF
}

# put everything back on exit or error
function finish {
  if [ ! -e JenkinsResults01.xml ]; then
    summary1
  fi
}
trap finish EXIT ERR

# golint pipeline always exits 0, run it first
echo $(date +'%F %T%z') LINT start
LINT_RESULT=FAILED
golint cmd/... pkg/... | tee golint.txt
if [ ! -s "golint.txt" ]; then
  LINT_RESULT=succeeded
fi
echo $(date +'%F %T%z') LINT complete

echo $(date +'%F %T%z') BUILD start
BUILD_RESULT=FAILED
time make build
BUILD_RESULT=succeeded
echo $(date +'%F %T%z') BUILD complete

# go vet pipeline also always exits 0
echo $(date +'%F %T%z') VET start
# simple printfuncs are case-independent
LogFuncs='criticalf,dbgf,debugf,errorf,fatalf,infof,noticef,panicf,warningf'
# TBD the following are all printf-like, but they don't end in "f" so go vet will not check them correctly
#CentraldFuncs='eexists,einvaliddata,emissingmsg,eupdateinvalidmsg,erequestinconflict'
#ReqFuncs='setrequesterror,setandupdaterequestmessage,setrequestmessage'
#UtilFuncs='util.NewTimestamedString,util.MsgList.Insert,util.Controller.Message,util.Controller.ReplaceMessage'
VET_RESULT=FAILED
go vet -all -printfuncs "$LogFuncs" ./cmd/... ./pkg/... |& tee govet.txt
trimmed=`grep -v '^go: \(downloading\|extracting\|finding\)' govet.txt || exit 0`
if [ -z "$trimmed" ]; then
  VET_RESULT=succeeded
fi
echo $(date +'%F %T%z') VET complete

echo $(date +'%F %T%z') TEST start
TEST_RESULT=FAILED
time make test
TEST_RESULT=succeeded
echo $(date +'%F %T%z') TEST complete

echo $(date +'%F %T%z') SUMMARY start
summary1

cat > JenkinsResults02.xml << EOF
<section name="Go Code Coverage" fontcolor="">
  <table>
    <tr>
      <td value="Package" bgcolor="LightGray" fontcolor="black" fontattribute="bold" align="center"/>
      <td value="Coverage" bgcolor="LightGray" fontcolor="black" fontattribute="bold" align="center"/>
    </tr>
EOF
for d in `find cmd pkg -name cover.out -exec dirname '{}' \; | sort`; do
    f=$d/cover.out
    h=$d/cover.html
    go tool cover -html=$f -o $h
    p=$(go tool cover -func=$f | sed -nEe 's/^total.*statements.[[:space:]]+//p')
    if [ "$p" == '100.0%' ] && grep -q ' 0$' $f; then
      p='100.0% *'
    fi
    cat >> JenkinsResults02.xml << EOF
    <tr>
      <td value="$d" href="artifact/$h" align="left"/>
      <td value="$p" align="right"/>
    </tr>
EOF
done

cat >> JenkinsResults02.xml << EOF
  </table>
  <field name="" value="* Rounded. One or more lines of code were not covered" />
</section>
EOF
echo $(date +'%F %T%z') SUMMARY complete

if [ -s "golint.txt" ]; then
    echo LINT FAILED
    exit 1
fi
if [ "$VET_RESULT" = FAILED ]; then
    echo VET FAILED
    exit 1
fi

echo SUCCESS
exit 0
