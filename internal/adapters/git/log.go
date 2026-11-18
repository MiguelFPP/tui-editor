package git

import (
	"strings"
	"time"

	"github.com/MiguelFPP/tui-editor/internal/domain"
)

// commitFields is the number of NUL-separated metadata tokens of the log and show formats.
const commitFields = 6

// showFields is commitFields plus the message body.
const showFields = commitFields + 1

// parseCommits reads the NUL-separated history page in groups of six.
func parseCommits(raw []byte, limit int) domain.CommitPage {
	tokens := splitNUL(raw)
	page := domain.CommitPage{}

	for i := 0; i+commitFields <= len(tokens); i += commitFields {
		commit, ok := parseCommit(tokens[i], tokens[i+1], tokens[i+2], tokens[i+3], tokens[i+4], tokens[i+5])
		if !ok {
			break
		}
		if len(page.Commits) >= limit {
			page.Truncated = true

			break
		}

		page.Commits = append(page.Commits, commit)
	}

	return page
}

// parseCommit builds one commit, dropping the record separator git leaves before the next one.
func parseCommit(hash, short, author, email, date, subject string) (domain.Commit, bool) {
	commit := domain.Commit{
		Hash:      strings.TrimSpace(hash),
		ShortHash: strings.TrimSpace(short),
		Author:    strings.TrimSpace(author),
		Email:     strings.TrimSpace(email),
		Subject:   strings.TrimSpace(subject),
	}
	if !isHash(commit.Hash) {
		return domain.Commit{}, false
	}
	if created, err := time.Parse(time.RFC3339, strings.TrimSpace(date)); err == nil {
		commit.Date = created
	}

	return commit, true
}

// parseShow reads the seven metadata fields of `show` plus the patch that follows them.
func parseShow(raw []byte) domain.CommitDetail {
	parts := strings.SplitN(string(raw), "\x00", showFields+1)
	if len(parts) < showFields {
		return domain.CommitDetail{}
	}

	commit, _ := parseCommit(parts[0], parts[1], parts[2], parts[3], parts[4], parts[5])
	detail := domain.CommitDetail{Commit: commit, Body: strings.TrimRight(parts[6], "\r\n")}
	if len(parts) <= showFields {
		return detail
	}

	files := parsePatch(parts[showFields])
	if len(files) > domain.MaxDiffFiles {
		files, detail.Truncated = files[:domain.MaxDiffFiles], true
	}

	for _, file := range files {
		if file.Truncated {
			detail.Truncated = true
		}
	}
	detail.Files = files

	return detail
}
