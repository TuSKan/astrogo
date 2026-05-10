package spk

import "testing"

func TestParseHorizonsSummary(t *testing.T) {
	input := `
 Multiple major-bodies match string "A*"

  ID#      Name                               Designation  IAU/aliases/other   
  -------  ---------------------------------- -----------  ------------------- 
        0  Solar System Barycenter                         SSB                  
        1  Mercury Barycenter                                                   
        2  Venus Barycenter                                                     
        3  Earth-Moon Barycenter                           EMB                  
        4  Mars Barycenter                                                      
        5  Jupiter Barycenter                                                   
        6  Saturn Barycenter                                                    
        7  Uranus Barycenter                                                    
        8  Neptune Barycenter                                                   
        9  Pluto Barycenter                                                     
       31  SEMB-L1                                         Lagrange             
       32  SEMB-L2                                         Lagrange             
       34  SEMB-L4                                         Lagrange             
       35  SEMB-L5                                         Lagrange             
      301  Moon                                            Luna                 
      399  Earth                                           Geocenter    
`

	targets, err := parseHorizonsResult(input)
	if err != nil {
		t.Fatalf("ParseHorizonsSummary failed: %v", err)
	}

	if len(targets) != 16 {
		t.Errorf("expected 16 targets, got %d", len(targets))
	}

	tests := []struct {
		id   string
		name string
		idx  int
	}{
		{"0", "Solar System Barycenter", 0},
		{"301", "Moon", 14},
		{"399", "Earth", 15},
	}

	for _, tt := range tests {
		if targets[tt.idx].ID != tt.id {
			t.Errorf("target[%d] ID = %s, want %s", tt.idx, targets[tt.idx].ID, tt.id)
		}

		if targets[tt.idx].Name != tt.name {
			t.Errorf("target[%d] Name = %s, want %s", tt.idx, targets[tt.idx].Name, tt.name)
		}
	}
}
