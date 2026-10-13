package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestMrTokenSubmissionIsNotDuplicatedAfterUncertainFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_submit_no_retry", true)
	for _, status := range []int{429, 500, 502, 504} {
		assert.False(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: status}, 3))
	}
}
