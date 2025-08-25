package polyjson_test

import (
	"testing"

	streammuxtypes "github.com/xaionaro-go/avpipeline/preset/streammux/types"
	"github.com/xaionaro-go/polyjson"
)

func TestCase0(t *testing.T) {
	polyjson.RegisterType(streammuxtypes.AutoBitrateCalculatorThresholds{})
	polyjson.RegisterType(streammuxtypes.AutoBitrateCalculatorLogK{})

	var calculator streammuxtypes.AutoBitRateCalculator
	polyjson.UnmarshalWithTypeIDs([]byte(`{
  "Inertia": 0.7,
  "MovingAverage": {
    "./avpipeline/indicator.MAMA[float64]": {
      "FastLimit": 0.3,
      "SlowLimit": 0.05
    }
  },
  "QueueOptimal": 100
}`), &calculator, polyjson.TypeRegistry())
}
