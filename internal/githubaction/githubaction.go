package githubaction

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/wader/bump/internal/bump"
	"github.com/wader/bump/internal/filter/all"
	"github.com/wader/bump/internal/github"
)

// CheckTemplateReplaceFn builds a function for doing template replacing for check
func CheckTemplateReplaceFn(c *bump.Check) func(s string) (string, error) {
	varReplacer := strings.NewReplacer(
		"$NAME", c.Name,
		"$LATEST", c.Latest,
		// TODO: this might be wrong if there are multiple current versions
		"$CURRENT", c.Currents[0].Version,
	)

	var currentVersions []string
	for _, c := range c.Currents {
		currentVersions = append(currentVersions, c.Version)
	}
	var messages []string
	for _, m := range c.Messages {
		messages = append(messages, varReplacer.Replace(m.Message))
	}
	type link struct {
		Title string
		URL   string
	}
	var links []link
	for _, l := range c.Links {
		links = append(links, link{
			Title: varReplacer.Replace(l.Title),
			URL:   varReplacer.Replace(l.URL),
		})
	}

	tmplData := struct {
		Name     string
		Current  []string
		Messages []string
		Latest   string
		Links    []link
	}{
		Name:     c.Name,
		Current:  currentVersions,
		Messages: messages,
		Latest:   c.Latest,
		Links:    links,
	}

	return func(s string) (string, error) {
		tmpl := template.New("")
		tmpl = tmpl.Funcs(template.FuncMap{
			"join": strings.Join,
		})
		tmpl, err := tmpl.Parse(s)
		if err != nil {
			return "", err
		}

		execBuf := &bytes.Buffer{}
		err = tmpl.Execute(execBuf, tmplData)
		if err != nil {
			return "", err
		}

		return execBuf.String(), nil
	}
}

// Command is a github action interface to bump packages
type Command struct {
	GHClient *github.Client
	OS       bump.OS
}

// Run bump in a github action environment
func (cmd Command) Run() []error {
	errs := cmd.run()
	for _, err := range errs {
		fmt.Fprintln(cmd.OS.Stderr(), err)
	}

	return errs
}

func (cmd Command) runExecs(argss [][]string) error {
	for _, args := range argss {
		fmt.Printf("> %s\n", strings.Join(args, " "))
		if err := cmd.OS.Exec(args, nil); err != nil {
			return err
		}
	}
	return nil
}

func (cmd Command) run() []error {
	ae, err := github.NewActionEnv(cmd.OS.Getenv, cmd.GHClient)
	if err != nil {
		return []error{err}
	}
	// TODO: used in tests
	ae.Client.BaseURL = cmd.OS.Getenv("GITHUB_API_URL")

	if ae.SHA == "" {
		return []error{fmt.Errorf("GITHUB_SHA not set")}
	}

	files, _ := ae.Input("bump_files")
	var bumpfile,
		titleTemplate,
		commitBodyTemplate,
		prBodyTemplate,
		branchTemplate,
		userName,
		userEmail string
	for _, v := range []struct {
		s *string
		n string
	}{
		{&bumpfile, "bumpfile"},
		{&titleTemplate, "title_template"},
		{&commitBodyTemplate, "commit_body_template"},
		{&prBodyTemplate, "pr_body_template"},
		{&branchTemplate, "branch_template"},
		{&userName, "user_name"},
		{&userEmail, "user_email"},
	} {
		s, err := ae.Input(v.n)
		if err != nil {
			return []error{err}
		}
		*v.s = s
	}

	pushURL := fmt.Sprintf("https://%s:%s@github.com/%s.git", ae.Actor, ae.Client.Token, ae.Repository)
	err = cmd.runExecs([][]string{
		{"git", "config", "--global", "user.name", userName},
		{"git", "config", "--global", "user.email", userEmail},
		{"git", "remote", "set-url", "--push", "origin", pushURL},
	})
	if err != nil {
		return []error{err}
	}

	// TODO: whitespace in filenames
	filesParts := strings.Fields(files)
	bfs, errs := bump.NewBumpFileSet(cmd.OS, all.Filters(), bumpfile, filesParts)
	if errs != nil {
		return errs
	}

	for _, c := range bfs.Checks {
		// only concider this check for update actions
		bfs.SkipCheckFn = func(skipC *bump.Check) bool {
			return skipC.Name != c.Name
		}

		ua, errs := bfs.UpdateActions()
		if errs != nil {
			return errs
		}

		fmt.Printf("Checking %s\n", c.Name)

		if !c.HasUpdate() {
			fmt.Printf("  No updates\n")

			// TODO: close if PR is open?
			continue
		}

		fmt.Printf("  Updateable to %s\n", c.Latest)

		templateReplacerFn := CheckTemplateReplaceFn(c)

		branchName, err := templateReplacerFn(branchTemplate)
		if err != nil {
			return []error{fmt.Errorf("branch template error: %w", err)}
		}
		if err := github.IsValidBranchName(branchName); err != nil {
			return []error{fmt.Errorf("branch name %q is invalid: %w", branchName, err)}
		}

		prs, err := ae.RepoRef.ListPullRequest("state", "all", "head", ae.Owner+":"+branchName)
		if err != nil {
			return []error{err}
		}

		// there is already an open or closed PR for this update
		if len(prs) > 0 {
			fmt.Printf("  Open or closed PR %d %s already exists\n",
				prs[0].Number, ae.Owner+":"+branchName)

			// TODO: do get pull request and check for mergable and rerun/close if needed?
			continue
		}

		// reset HEAD back to triggering commit before each PR
		err = cmd.runExecs([][]string{{"git", "reset", "--hard", ae.SHA}})
		if err != nil {
			return []error{err}
		}

		for _, fc := range ua.FileChanges {
			if err := cmd.OS.WriteFile(fc.File.Name, []byte(fc.NewText)); err != nil {
				return []error{err}
			}

			fmt.Printf("  Wrote change to %s\n", fc.File.Name)
		}

		for _, rs := range ua.RunShells {
			if err := cmd.OS.Shell(rs.Cmd, rs.Env); err != nil {
				return []error{fmt.Errorf("%s: shell: %s: %w", rs.Check.Name, rs.Cmd, err)}
			}
		}

		title, err := templateReplacerFn(titleTemplate)
		if err != nil {
			return []error{fmt.Errorf("title template error: %w", err)}
		}
		commitBody, err := templateReplacerFn(commitBodyTemplate)
		if err != nil {
			return []error{fmt.Errorf("title template error: %w", err)}
		}
		prBody, err := templateReplacerFn(prBodyTemplate)
		if err != nil {
			return []error{fmt.Errorf("title template error: %w", err)}
		}

		err = cmd.runExecs([][]string{
			{"git", "diff"},
			{"git", "add", "--update"},
			{"git", "commit", "--message", title, "--message", commitBody},
			// force so if for some reason there was an existing closed update PR with the same name
			{"git", "push", "--force", "origin", "HEAD:refs/heads/" + branchName},
		})
		if err != nil {
			return []error{err}
		}

		fmt.Printf("  Committed and pushed\n")

		newPr, err := ae.RepoRef.CreatePullRequest(github.NewPullRequest{
			Base:  ae.Ref,
			Head:  ae.Owner + ":" + branchName,
			Title: title,
			Body:  &prBody,
		})
		if err != nil {
			return []error{err}
		}

		fmt.Printf("  Created PR %s\n", newPr.URL)
	}

	return nil
}
