package githubservice

import (
	"github.com/google/go-github/github"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"log"
	"sort"
	"strings"
	"time"
)

type GithubService struct {
	PersonalAccessToken string
}

func New(personalAccessToken string) *GithubService {
	g := GithubService{
		PersonalAccessToken: personalAccessToken,
	}
	return &g
}

type TokenSource struct {
	AccessToken string
}

func (t *TokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

func (g *GithubService) obtainAuthenticatedGithubClient() (c *github.Client) {
	tokenSource := &TokenSource{
		AccessToken: g.PersonalAccessToken,
	}
	oauthClient := oauth2.NewClient(context.TODO(), tokenSource)
	return github.NewClient(oauthClient)
}

func (g *GithubService) loadIssuesForAssignee(owner string, assignee string) ([]github.Issue, error) {
	var client = g.obtainAuthenticatedGithubClient()
	var all []github.Issue
	var e error
	opt := &github.SearchOptions{

		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		issueSearchResults, resp, err := client.Search.Issues("user:"+owner+" assignee:"+assignee, opt)

		if err != nil {
			e = err
			break
		}

		all = append(all, issueSearchResults.Issues...)

		if resp.NextPage == 0 {
			break
		}

		opt.ListOptions.Page = resp.NextPage
	}

	return all, e
}

func (g *GithubService) loadIssuesForRepo(owner string, repo string, assigned string) ([]github.Issue, error) {
	var client = g.obtainAuthenticatedGithubClient()
	var allIssues []github.Issue
	var e error
	opt := &github.IssueListByRepoOptions{
		Assignee:    assigned,
		ListOptions: github.ListOptions{PerPage: 500},
	}

	for {
		issues, resp, err := client.Issues.ListByRepo(owner, repo, opt)

		if err != nil {
			e = err
			break
		}

		allIssues = append(allIssues, issues...)

		if resp.NextPage == 0 {
			break
		}

		opt.ListOptions.Page = resp.NextPage
	}

	return allIssues, e
}

func (g *GithubService) loadCommitsForRepo(owner string, repo string, committer string, timeLimit time.Time) ([]github.RepositoryCommit, error) {
	var client = g.obtainAuthenticatedGithubClient()
	var allCommits []github.RepositoryCommit
	var e error
	opt := &github.CommitsListOptions{
		Since:       timeLimit,
		ListOptions: github.ListOptions{PerPage: 500},
	}

	for {
		repositoryCommits, resp, err := client.Repositories.ListCommits(owner, repo, opt)

		if err != nil {
			e = err
			break
		}

		allCommits = append(allCommits, repositoryCommits...)

		if resp.NextPage == 0 {
			break
		}

		opt.ListOptions.Page = resp.NextPage
	}

	return allCommits, e
}

func (g *GithubService) loadReposForOrganization(owner string) ([]github.Repository, error) {
	var client = g.obtainAuthenticatedGithubClient()
	var allRepos []github.Repository
	var e error
	opt := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(owner, opt)

		if err != nil {
			e = err
			break
		}

		allRepos = append(allRepos, repos...)

		if resp.NextPage == 0 {
			break
		}

		opt.ListOptions.Page = resp.NextPage
	}

	return allRepos, e
}

func (g *GithubService) loadPRsForRepo(owner string, repo string) ([]github.PullRequest, error) {
	var client = g.obtainAuthenticatedGithubClient()
	var allPRs []github.PullRequest
	var e error
	opt := &github.PullRequestListOptions{
		State: "open",
		// TODO: These params should be available but sadly they don't pass the compiler
		//		Sort: 		"long-running",
		//		Direction: 	"desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		pullRequests, resp, err := client.PullRequests.List(owner, repo, opt)
		if err != nil {
			e = err
			break
		}

		allPRs = append(allPRs, pullRequests...)

		if resp.NextPage == 0 {
			break
		}

		opt.ListOptions.Page = resp.NextPage
	}

	return allPRs, e
}

func (g *GithubService) loadCommitsFromAllRepoPRs(owner string, repo string, timeLimit time.Time) ([]github.RepositoryCommit, error) {
	var client = g.obtainAuthenticatedGithubClient()
	var allPRCommits []github.RepositoryCommit
	var e error
	opt := &github.PullRequestListOptions{
		State:       "open,closed",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	remainingPRsAreOlder := false

	for {
		pullRequests, resp, err := client.PullRequests.List(owner, repo, opt)
		if err != nil {
			e = err
			break
		}

		for _, pullRequest := range pullRequests {

			if timeLimit.After(*pullRequest.CreatedAt) && timeLimit.After(*pullRequest.UpdatedAt) {
				//PR is older than time box. Assuming a sorted list it is safe to stop processing.
				remainingPRsAreOlder = true
				break
			}

			prOpt := &github.ListOptions{
				PerPage: 100,
			}
			prCommits, prResp, err := client.PullRequests.ListCommits(owner, repo, *pullRequest.Number, prOpt)

			if err != nil {
				e = err
				continue
			}

			for _, prCommit := range prCommits {
				allPRCommits = append(allPRCommits, prCommit)
			}

			if prResp.NextPage == 0 {
				continue
			}

			prOpt.Page = prResp.NextPage
		}

		if resp.NextPage == 0 || remainingPRsAreOlder {
			break
		}

		opt.ListOptions.Page = resp.NextPage
	}

	return allPRCommits, e
}

func (g *GithubService) loadActiveReposForOrganization(owner string, days int) ([]github.Repository, error) {
	var allRepos []github.Repository
	var activeRepos []github.Repository
	var e error

	allRepos, err := g.loadReposForOrganization(owner)
	if err != nil {
		e = err
	}

	for _, repo := range allRepos {
		if time.Since(repo.PushedAt.Time).Hours() <= float64(days)*24 {
			activeRepos = append(activeRepos, repo)
		}
	}
	sort.Sort(RepositoryNameSorter(activeRepos))

	return activeRepos, e
}

func (g *GithubService) loadOpenPRsForOrganization(owner string, daysPROpen int, daysSinceLastProjectActivity int) ([]github.PullRequest, error) {
	var activeRepos []github.Repository
	var e error

	activeRepos, err := g.loadActiveReposForOrganization(owner, daysSinceLastProjectActivity)
	if err != nil {
		e = err
	}

	var allOpenPRs []github.PullRequest
	for _, repo := range activeRepos {
		pullRequests, err := g.loadPRsForRepo(owner, *repo.Name)
		if err != nil {
			e = err
			break
		}

		for _, pullRequest := range pullRequests {
			var numberOfDays = time.Since(*pullRequest.CreatedAt).Hours() / 24
			if numberOfDays < float64(daysPROpen) {
				log.Printf("Skipping PR open for %f days", numberOfDays)
				break // These PRs are too new for us to care about
			} else {
				log.Printf("Adding PR open for %f days", numberOfDays)
				allOpenPRs = append(allOpenPRs, pullRequest)
			}
		}
	}

	sort.Sort(PROpenDurationSorter(allOpenPRs))

	return allOpenPRs, e
}

// RepositoryNameSorter sorts Repository by name.
type RepositoryNameSorter []github.Repository

func (a RepositoryNameSorter) Len() int      { return len(a) }
func (a RepositoryNameSorter) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a RepositoryNameSorter) Less(i, j int) bool {
	return strings.ToLower(*a[i].Name) < strings.ToLower(*a[j].Name)
}

// PROpenDurationSorter sorts PRs by the length of time they are open.
type PROpenDurationSorter []github.PullRequest

func (a PROpenDurationSorter) Len() int           { return len(a) }
func (a PROpenDurationSorter) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a PROpenDurationSorter) Less(i, j int) bool { return (*a[i].CreatedAt).Before(*a[j].CreatedAt) }

func (g *GithubService) makeIssueList(owner string, repo string, assigned string, lambda func(github.Issue) bool) ([]github.Issue, error) {

	issues, err := g.loadIssuesForRepo(owner, repo, assigned)

	if err != nil {
		return nil, err
	}

	var sprintIssues []github.Issue

	for _, issue := range issues {

		// Backlog items are ones that don't match any of the other categories
		if issue.Labels != nil {
			if lambda(issue) {
				sprintIssues = append(sprintIssues, issue)
			}
		}
	}

	return sprintIssues, err
}

func (g *GithubService) makeCommitsList(owner string, repo string, committer string, lambda func(github.RepositoryCommit, []github.RepositoryCommit) bool, days int) (map[string][]github.RepositoryCommit, int, error) {

	totalCommits := 0
	repoToMasterCommits := make(map[string][]github.RepositoryCommit)

	if repo == "" {
		//summary of commits from all repos
		repositories, err := g.loadActiveReposForOrganization(owner, days)
		if err != nil {
			return nil, 0, err
		}

		for _, repository := range repositories {
			repoName := *repository.Name
			masterCommits, totalRepoCommits, err := g.masterCommitsForSingleRepo(owner, repoName, committer, lambda, days)

			if err != nil {
				return nil, 0, err
			}

			if len(masterCommits) > 0 {
				repoToMasterCommits[repoName] = masterCommits
				totalCommits += totalRepoCommits
			}
		}
	} else {
		//single repo query
		masterCommits, totalRepoCommits, err := g.masterCommitsForSingleRepo(owner, repo, committer, lambda, days)

		if err != nil {
			return nil, 0, err
		}

		repoToMasterCommits[repo] = masterCommits
		totalCommits += totalRepoCommits
	}

	return repoToMasterCommits, totalCommits, nil
}

func (g *GithubService) masterCommitsForSingleRepo(owner string, repo string, committer string, lambda func(github.RepositoryCommit, []github.RepositoryCommit) bool, days int) ([]github.RepositoryCommit, int, error) {

	var timeLimit = time.Now().AddDate(0, 0, -days)

	commits, err := g.loadCommitsForRepo(owner, repo, committer, timeLimit)
	allPRCommits, err := g.loadCommitsFromAllRepoPRs(owner, repo, timeLimit)

	if err != nil {
		return nil, 0, err
	}

	masterCommits := make([]github.RepositoryCommit, 0)

	for _, commit := range commits {
		// Do not include merge commits
		// Only include commits that are not part of a PR
		if len(commit.Parents) <= 1 && !lambda(commit, allPRCommits) {
			masterCommits = append(masterCommits, commit)
		}
	}

	return masterCommits, len(commits), err
}

func (g *GithubService) AssignedTo(owner string, repo string, login string) ([]github.Issue, error) {
	if repo == "*" {
		return g.loadIssuesForAssignee(owner, login)

	} else {
		return g.makeIssueList(owner, repo, login, g.any)
	}
}

func (g *GithubService) Sprint(owner string, repo string) ([]github.Issue, error) {
	return g.makeIssueList(owner, repo, "", g.isSprintItem)
}

func (g *GithubService) InProgress(owner string, repo string) ([]github.Issue, error) {
	return g.makeIssueList(owner, repo, "", g.isInProgress)
}

func (g *GithubService) ReadyForQA(owner string, repo string) ([]github.Issue, error) {
	return g.makeIssueList(owner, repo, "", g.isReadyForQA)
}

func (g *GithubService) QAPass(owner string, repo string) ([]github.Issue, error) {
	return g.makeIssueList(owner, repo, "", g.isQAPass)
}

func (g *GithubService) Backlog(owner string, repo string) ([]github.Issue, error) {
	return g.makeIssueList(owner, repo, "", g.isBacklogItem)
}

func (g *GithubService) ReadyForReview(owner string, repo string) ([]github.Issue, error) {
	return g.makeIssueList(owner, repo, "", g.isReadyForReview)
}

func (g *GithubService) OpenPullRequests(owner string, daysPROpen int, daysSinceLastProjectActivity int) ([]github.PullRequest, error) {
	return g.loadOpenPRsForOrganization(owner, daysPROpen, daysSinceLastProjectActivity)
}

func (g *GithubService) CommitsToMaster(owner string, repo string, days int) (map[string][]github.RepositoryCommit, int, error) {
	return g.makeCommitsList(owner, repo, "", g.isCommitInList, days)
}

func (g *GithubService) getLabelString(labels []github.Label) string {
	var retval string
	for _, label := range labels {
		retval += strings.ToLower(*label.Name) + " "
	}
	return retval
}

func (g *GithubService) any(issue github.Issue) bool {
	return true
}

func (g *GithubService) isBacklogItem(issue github.Issue) bool {

	if g.isSprintItem(issue) || g.isInProgress(issue) || g.isReadyForQA(issue) || g.isQAPass(issue) || g.isDone(issue) {
		return false
	} else {
		return true
	}
}

func (g *GithubService) isProductBacklogItem(issue github.Issue) bool {
	label := g.getLabelString(issue.Labels)
	return strings.Contains(label, "product") && strings.Contains(label, "backlog")
}

func (g *GithubService) isSprintItem(issue github.Issue) bool {
	label := g.getLabelString(issue.Labels)
	return strings.Contains(label, "sprint")
}

func (g *GithubService) isInProgress(issue github.Issue) bool {
	label := g.getLabelString(issue.Labels)
	return strings.Contains(label, "in") && strings.Contains(label, "progress")
}

func (g *GithubService) isReadyForQA(issue github.Issue) bool {
	label := g.getLabelString(issue.Labels)
	return strings.Contains(label, "ready") && strings.Contains(label, "for") && strings.Contains(label, "qa")
}

func (g *GithubService) isReadyForReview(issue github.Issue) bool {
	label := g.getLabelString(issue.Labels)
	return strings.Contains(label, "ready") && strings.Contains(label, "for") && strings.Contains(label, "review")
}

func (g *GithubService) isQAPass(issue github.Issue) bool {
	label := g.getLabelString(issue.Labels)
	return strings.Contains(label, "qa") && strings.Contains(label, "pass")
}

func (g *GithubService) isDone(issue github.Issue) bool {
	label := g.getLabelString(issue.Labels)
	return strings.Contains(label, "done")
}

func (g *GithubService) isCommitInList(commit github.RepositoryCommit, commitList []github.RepositoryCommit) bool {

	for _, listCommit := range commitList {
		if *commit.SHA == *listCommit.SHA {
			return true
		}
	}

	return false
}
