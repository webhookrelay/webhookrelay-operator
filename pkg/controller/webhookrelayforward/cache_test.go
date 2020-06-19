package webhookrelayforward

import (
	"testing"

	"github.com/webhookrelay/webhookrelay-go"
	"gotest.tools/assert"
)

func TestCache_AddOutput(t *testing.T) {

	c := newBucketsCache()

	t.Run("TestAddToEmpty", func(t *testing.T) {
		c.AddOutput(&webhookrelay.Output{ID: "foo"})

		assert.Equal(t, 0, len(c.items))

	})

	t.Run("TestAddBucket", func(t *testing.T) {
		c.Add(&webhookrelay.Bucket{ID: "foo", Name: "b-1"})

		c.AddOutput(&webhookrelay.Output{ID: "foo", BucketID: "foo"})

		assert.Equal(t, 1, len(c.items))
		assert.Equal(t, 1, len(c.items["b-1"].Outputs))
	})

	t.Run("TestUpdateBucket", func(t *testing.T) {
		c.AddOutput(&webhookrelay.Output{ID: "foo", BucketID: "foo", Name: "o-1"})

		name := c.items["b-1"].Outputs[0].Name
		assert.Equal(t, 1, len(c.items))
		assert.Equal(t, 1, len(c.items["b-1"].Outputs))
		assert.Equal(t, "o-1", name)
	})
}
