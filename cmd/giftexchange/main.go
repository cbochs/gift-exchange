package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cbochs/gift-exchange/internal/dto"
	ge "github.com/cbochs/gift-exchange/lib"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: giftexchange <solve|validate|analyze> [flags]")
		return 1
	}
	switch args[0] {
	case "solve":
		return cmdSolve(args[1:], stdin, stdout, stderr)
	case "validate":
		return cmdValidate(args[1:], stdin, stdout, stderr)
	case "analyze":
		return cmdAnalyze(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %q\n", args[0])
		return 1
	}
}

// ---------------------------------------------------------------------------
// JSON schema types
// ---------------------------------------------------------------------------

// inputOptions is the JSON representation of solver options.
// Uses snake_case and omits Timeout (not exposed via CLI JSON).
type inputOptions struct {
	MaxSolutions int   `json:"max_solutions,omitempty"`
	Seed         int64 `json:"seed,omitempty"`
}

func (o inputOptions) toLibOptions() ge.Options {
	n := o.MaxSolutions
	if n <= 0 {
		n = ge.DefaultMaxSolutions
	}
	return ge.Options{
		MaxSolutions: n,
		Seed:         o.Seed,
	}
}

// inputDoc is the unified JSON schema for both input and round-trip output.
// On output, Solutions and Feasible are populated; they are silently ignored
// on re-read, making the output a valid input for a subsequent run.
type inputDoc struct {
	Participants  []dto.ParticipantDTO  `json:"participants"`
	Blocks        []dto.BlockDTO        `json:"blocks,omitempty"`
	Relationships []dto.RelationshipDTO `json:"relationships,omitempty"`
	BlockGroups   []dto.BlockGroupDTO   `json:"block_groups,omitempty"`
	Options       inputOptions          `json:"options"`
	// Round-trip fields (written on output, ignored when re-used as input).
	Solutions []dto.SolutionDTO `json:"solutions,omitempty"`
	Feasible  *bool             `json:"feasible,omitempty"`
}

func (d *inputDoc) problem() ge.Problem {
	return dto.BuildProblem(d.Participants, d.Blocks, d.Relationships, d.BlockGroups)
}

// ---------------------------------------------------------------------------
// Input reading
// ---------------------------------------------------------------------------

func readInput(path string, stdin io.Reader) (*inputDoc, error) {
	if path == "" {
		return nil, fmt.Errorf("--input flag is required (use - for stdin)")
	}
	var r io.Reader
	if path == "-" {
		r = stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	var doc inputDoc
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}
	return &doc, nil
}

// ---------------------------------------------------------------------------
// solve subcommand
// ---------------------------------------------------------------------------

func cmdSolve(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("solve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "input JSON file (or - for stdin)")
	asJSON := fs.Bool("json", false, "output as JSON")
	seedOver := fs.Int64("seed", 0, "override seed from input (0 = use input seed)")
	nOver := fs.Int("n", 0, "override max_solutions from input (0 = use input value)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	doc, err := readInput(*inputPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	opts := doc.Options.toLibOptions()
	if *seedOver != 0 {
		opts.Seed = *seedOver
	}
	if *nOver > 0 {
		opts.MaxSolutions = *nOver
	}
	// Materialize the seed before calling Solve so the JSON output embeds the
	// actual seed used, enabling exact round-trip reproducibility.
	if opts.Seed == 0 {
		opts.Seed = ge.NewSeed()
	}
	// Keep doc.Options in sync with the effective values for round-trip output.
	doc.Options.Seed = opts.Seed
	doc.Options.MaxSolutions = opts.MaxSolutions

	solutions, err := ge.Solve(context.Background(), doc.problem(), opts)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		if errors.Is(err, ge.ErrInfeasible) {
			return 2
		}
		return 1
	}

	if *asJSON {
		return writeJSONOutput(stdout, stderr, doc, solutions)
	}
	return writeHuman(stdout, solutions, opts.Seed)
}

// ---------------------------------------------------------------------------
// validate subcommand
// ---------------------------------------------------------------------------

func cmdValidate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "input JSON file (or - for stdin)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	doc, err := readInput(*inputPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	prob := doc.problem()
	if err := ge.Validate(prob); err != nil {
		fmt.Fprintln(stderr, "invalid:", err)
		return 1
	}

	fmt.Fprintf(stdout, "Input is valid.\nParticipants: %d\nBlocks: %d\n",
		len(prob.Participants), len(prob.Blocks))
	return 0
}

// ---------------------------------------------------------------------------
// analyze subcommand
// ---------------------------------------------------------------------------

func cmdAnalyze(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inputPath := fs.String("input", "", "input JSON file (or - for stdin)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	doc, err := readInput(*inputPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	info, err := ge.Analyze(context.Background(), doc.problem())
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		if errors.Is(err, ge.ErrInfeasible) {
			return 2
		}
		return 1
	}

	hamiltonian := "no"
	if info.HamiltonianPossible {
		hamiltonian = "yes"
	}
	fmt.Fprintf(stdout, "Participants:  %d\n", info.ParticipantCount)
	fmt.Fprintf(stdout, "Edges:         %d of %d possible (%.1f%% density)\n",
		info.EdgeCount, info.MaxEdgeCount, info.Density*100)
	fmt.Fprintf(stdout, "Hamiltonian:   %s\n", hamiltonian)
	return 0
}

// ---------------------------------------------------------------------------
// Output formatters
// ---------------------------------------------------------------------------

func writeJSONOutput(stdout, stderr io.Writer, doc *inputDoc, solutions []ge.Solution) int {
	feasible := true
	doc.Solutions = dto.SolutionsFromLib(solutions)
	doc.Feasible = &feasible
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintln(stderr, "json encode error:", err)
		return 1
	}
	return 0
}

func writeHuman(w io.Writer, solutions []ge.Solution, seed int64) int {
	fmt.Fprintf(w, "Seed: %d\nSolutions found: %d\n\n", seed, len(solutions))
	for i, s := range solutions {
		fmt.Fprintf(w, "=== Solution %d ===\n", i+1)
		fmt.Fprintf(w, "Score: min_cycle=%d  cycles=%d  max_cycle=%d\n",
			s.Score.MinCycleLen, s.Score.NumCycles, s.Score.MaxCycleLen)
		fmt.Fprintln(w, "Cycles:")
		for j, c := range s.Cycles {
			fmt.Fprintf(w, "  [%d] %s → %s\n", j+1, strings.Join([]string(c), " → "), c[0])
		}
		fmt.Fprintln(w)
	}
	return 0
}
