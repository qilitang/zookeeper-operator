/*


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

package status

import (
	"bytes"
	"fmt"
)

/*
进度条
采用二叉树：孩子兄弟表示法来存储
*/
type ProgressStep struct {
	Name    string
	Type    string
	Message string
	Next    *ProgressStep
	Child   *ProgressStep
}

func NewProgress(n, t, m string) *ProgressStep {
	return &ProgressStep{Name: n, Type: t, Message: m}
}

func (t *ProgressStep) AddNext(step *ProgressStep) *ProgressStep {
	var last *ProgressStep
	last = t.Next
	if t.Next == nil {
		t.Next = step
		return t
	}
	for last.Next != nil {
		last = last.Next
	}
	last.Next = step
	return t
}

func (t *ProgressStep) AddChild(step *ProgressStep) *ProgressStep {
	if t.Child == nil {
		t.Child = step
		return t
	}
	t.Child.AddNext(step) // 孩子的兄弟也是它的孩子
	return t
}

/*
将进度条转成可视化的文本信息。
例：
1.[A,A0,AAA]
1.1.[B,B0,BBB]
1.1.1.[C,C0,CCC]
1.2.[B2,B0,bbb]
1.3.[B3,B0,ddd]
2.[A2,A0,a222]
3.[A3,A0,a333]
例子说明：
总共有3大步骤：A，A2，A3
其中A有3个子步骤：B，B2，B3

	B有1个子步骤：C

数字的编号类似word目录编号

prefix为描述前缀，会加在每一行最前面，idx为开始序号（初始传入0表示从1开始）
*/
func (t *ProgressStep) ToString(prefix string, idx int) string {
	buffer := bytes.Buffer{}

	idx++
	buffer.WriteString(fmt.Sprintf("%s%d.[%s,%s,%s]\n", prefix, idx, t.Name, t.Type, t.Message))

	// do child
	if t.Child != nil {
		buffer.WriteString(t.Child.ToString(fmt.Sprintf("%s%d.", prefix, idx), 0))
	}

	// do next
	if t.Next != nil {
		buffer.WriteString(t.Next.ToString(prefix, idx))
	}

	return buffer.String()
}
