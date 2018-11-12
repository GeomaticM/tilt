package dockerfile

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/docker/distribution/reference"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/windmilleng/tilt/internal/model"
)

var ErrAddInDockerfile = errors.New("base Dockerfile contains an ADD/COPY, " +
	"which is not currently supported -- move this to an add() call in your Tiltfile")

type Dockerfile string

// DockerfileFromExisting creates a new Dockerfile that uses the supplied image
// as its base image with a FROM statement. This is necessary for iterative
// Docker builds.
func FromExisting(existing reference.NamedTagged) Dockerfile {
	return Dockerfile(fmt.Sprintf("FROM %s", existing.String()))
}

func (d Dockerfile) Append(df Dockerfile) Dockerfile {
	return d.Join(string(df))
}

func (d Dockerfile) Join(s string) Dockerfile {
	return Dockerfile(fmt.Sprintf("%s\n%s", d, s))
}

func (d Dockerfile) WithLabel(label Label, val LabelValue) Dockerfile {
	return d.Join(fmt.Sprintf("LABEL %q=%q", label, val))
}

func (d Dockerfile) AddAll() Dockerfile {
	return d.Join("ADD . /")
}

func (d Dockerfile) Run(cmd model.Cmd) Dockerfile {
	return d.Join(cmd.RunStr())
}

func (d Dockerfile) Entrypoint(cmd model.Cmd) Dockerfile {
	return d.Join(cmd.EntrypointStr())
}

func (d Dockerfile) RmPaths(pathsToRm []string) Dockerfile {
	if len(pathsToRm) == 0 {
		return d
	}

	// Add 'rm' statements; if changed path was deleted locally, remove if from container
	rmCmd := strings.Builder{}
	rmCmd.WriteString("rm -rf")
	for _, p := range pathsToRm {
		rmCmd.WriteString(fmt.Sprintf(" %s", p))
	}
	return d.Join(fmt.Sprintf("RUN %s", rmCmd.String()))
}

// If possible, split this dockerfile into two parts:
// a base dockerfile (without any adds/copys) and a "iterative" dockerfile.
// Useful for constructing the directory cache.
// Returns false if we can't split it.
func (d Dockerfile) SplitIntoBaseDockerfile() (Dockerfile, Dockerfile, bool) {
	asString := string(d)
	result, err := parser.Parse(bytes.NewBufferString(asString))
	if err != nil {
		return "", "", false
	}

	// TODO(nick): Right now, we just check for the first ADD/COPY
	// and split after than. This is Good Enough (tm) for cache dirs.
	// In the future, we would need to understand multi-stage builds.
	//
	// TODO(nick): This would be easier if we could serialize df parser nodes
	// back into strings, but I haven't found an easy off-the-shelf
	// library for doing that.
	startLine := -1
	err = traverse(result.AST, func(node *parser.Node) error {
		switch strings.ToUpper(node.Value) {
		case "ADD", "COPY":
			if startLine == -1 {
				startLine = node.StartLine
			}
		}
		return nil
	})
	if err != nil {
		return "", "", false
	}

	// If there is no ADD, we're not sure what we're dealing with.
	if startLine == -1 {
		return "", "", false
	}

	lines := strings.Split(asString, "\n")

	// line numbers in dockerfile nodes are 1-based instead of 0-based
	baseDf := strings.Join(lines[:startLine-1], "\n")
	restDf := strings.Join(lines[startLine-1:], "\n")
	return Dockerfile(baseDf), Dockerfile(restDf), true
}

func (d Dockerfile) ValidateBaseDockerfile() error {
	result, err := parser.Parse(bytes.NewBufferString(string(d)))
	if err != nil {
		return fmt.Errorf("ValidateBaseDockerfile: %v", err)
	}

	err = traverse(result.AST, func(node *parser.Node) error {
		switch strings.ToUpper(node.Value) {
		case "ADD", "COPY":
			return ErrAddInDockerfile
		}
		return nil
	})
	return err
}

func (d Dockerfile) String() string {
	return string(d)
}

// Post-order traversal of the Dockerfile AST.
// Halts immediately on error.
func traverse(node *parser.Node, visit func(*parser.Node) error) error {
	for _, c := range node.Children {
		err := traverse(c, visit)
		if err != nil {
			return err
		}
	}
	return visit(node)
}