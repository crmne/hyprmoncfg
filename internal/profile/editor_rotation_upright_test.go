package profile

import "testing"

func TestRotationUprightSurvivesNormalButNotAnotherChoice(t *testing.T) {
	cases := []struct {
		name  string
		steps func(o *OutputConfig)
		want  int
	}{
		{"auto, normal, auto restores", func(o *OutputConfig) { choose(o, 0); auto(o) }, 3},
		{"auto, 90, auto replaces", func(o *OutputConfig) { choose(o, 1); auto(o) }, 1},
		{"auto, 90, normal, auto clears", func(o *OutputConfig) { choose(o, 1); choose(o, 0); auto(o) }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := OutputConfig{Transform: 3, RotationAuto: true}
			tc.steps(&output)
			if output.Transform != tc.want || !output.RotationAuto || output.RotationUpright != 0 {
				t.Fatalf("got %+v, want upright %d", output, tc.want)
			}
		})
	}
}

func choose(o *OutputConfig, transform int) {
	wasAuto, previous := o.RotationAuto, o.Transform
	o.Transform, o.RotationAuto = transform, false
	o.RememberRotationUpright(wasAuto, previous)
}

func auto(o *OutputConfig) {
	wasAuto, previous := o.RotationAuto, o.Transform
	o.RotationAuto = true
	o.RememberRotationUpright(wasAuto, previous)
}
