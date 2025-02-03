package baremetal

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

type testCase struct {
	name     string
	nodeList string
	nodeArr  []string
}

func createTestCases() []testCase {

	var testCases []testCase

	// Case 0
	var nodeArr0 []string
	for i := 1; i <= 4; i++ {
		nodeArr0 = append(nodeArr0, fmt.Sprintf("nodename-1-00%v", i))
	}
	nodeArr0 = append(nodeArr0, "nodename-1-007")
	for i := 91; i <= 99; i++ {
		nodeArr0 = append(nodeArr0, fmt.Sprintf("nodename-1-%v", i))
	}
	nodeArr0 = append(nodeArr0, "nodename-1-100")
	nodeArr0 = append(nodeArr0, "nodename-2-89")
	case0 := testCase{
		name:     "Case0",
		nodeList: "nodename-1-[001-004,007,91-99,100],nodename-2-89",
		nodeArr:  nodeArr0,
	}

	// Case 1
	var nodeArr1 []string
	prefix1 := "nodename-1-"
	for i := 1; i <= 4; i++ {
		nodeArr1 = append(nodeArr1, fmt.Sprintf("%v00%v", prefix1, i))
	}
	nodeArr1 = append(nodeArr1, fmt.Sprintf("%v007", prefix1))
	for i := 91; i <= 99; i++ {
		nodeArr1 = append(nodeArr1, fmt.Sprintf("%v%v", prefix1, i))
	}
	nodeArr1 = append(nodeArr1, fmt.Sprintf("%v100", prefix1))
	for i := 89; i <= 91; i++ {
		nodeArr1 = append(nodeArr1, fmt.Sprintf("nodename-2-%v", i))
	}
	case1 := testCase{
		name:     "Case1",
		nodeList: "nodename-1-[001-004,007,91-99,100],nodename-2-[89-91]",
		nodeArr:  nodeArr1,
	}

	// Case 2
	var nodeArr2 []string
	nodeArr2 = append(nodeArr2, "nodename-2-89")
	for i := 1; i <= 4; i++ {
		nodeArr2 = append(nodeArr2, fmt.Sprintf("nodename-1-00%v", i))
	}
	nodeArr2 = append(nodeArr2, "nodename-1-007")
	for i := 91; i <= 99; i++ {
		nodeArr2 = append(nodeArr2, fmt.Sprintf("nodename-1-%v", i))
	}
	nodeArr2 = append(nodeArr2, "nodename-1-100")
	case2 := testCase{
		name:     "Case2",
		nodeList: "nodename-2-89,nodename-1-[001-004,007,91-99,100]",
		nodeArr:  nodeArr2,
	}

	// Case 3
	var nodeArr3 []string

	for i := 1; i <= 4; i++ {
		nodeArr3 = append(nodeArr3, fmt.Sprintf("alpha-1-00%v", i))
	}
	nodeArr3 = append(nodeArr3, "alpha-1-007")
	for i := 91; i <= 99; i++ {
		nodeArr3 = append(nodeArr3, fmt.Sprintf("alpha-1-%v", i))
	}
	nodeArr3 = append(nodeArr3, "alpha-1-100")
	nodeArr3 = append(nodeArr3, "beta-2-89")

	case3 := testCase{
		name:     "Case3",
		nodeList: "alpha-1-[001-004,007,91-99,100],beta-2-89",
		nodeArr:  nodeArr3,
	}
	testCases = append(testCases, case0, case1, case2, case3)
	return testCases

}

func TestDecompressNodeNames(t *testing.T) {

	testCases := createTestCases()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := deCompressNodeNames(tc.nodeList)
			require.Equal(t, tc.nodeArr, res)

		})
	}

}
