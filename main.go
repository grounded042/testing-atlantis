package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

func main() {
	fmt.Println("Hello, World (1)!")

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	client := githubv4.NewClient(httpClient)

	err := discardReviews(client, Repo{
		Owner: "grounded042",
		Name:  "testing-atlantis",
	}, PullRequest{
		Num: 2,
	})
	if err != nil {
		fmt.Println("ERROR", err)
		return
	}
}

type Repo struct {
	Owner string
	Name  string
}

type PullRequest struct {
	Num int
}

type GithubReview struct {
	ID          githubv4.ID
	SubmittedAt githubv4.DateTime
	Author      struct {
		Login githubv4.String
	}
}

type GithubPRReviewSummary struct {
	ReviewDecision githubv4.String
	Reviews        []GithubReview
}

func getPRReviews(client *githubv4.Client, repo Repo, pull PullRequest) (GithubPRReviewSummary, error) {
	var query struct {
		Repository struct {
			PullRequest struct {
				ReviewDecision githubv4.String
				Reviews        struct {
					Nodes []GithubReview
					// contains pagination information
					PageInfo struct {
						EndCursor   githubv4.String
						HasNextPage githubv4.Boolean
					}
				} `graphql:"reviews(first: $entries, after: $reviewCursor, states: $reviewState)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":        githubv4.String(repo.Owner),
		"name":         githubv4.String(repo.Name),
		"number":       githubv4.Int(pull.Num),
		"entries":      githubv4.Int(10),
		"reviewState":  []githubv4.PullRequestReviewState{githubv4.PullRequestReviewStateApproved},
		"reviewCursor": (*githubv4.String)(nil), // initialize the reviewCursor with null
	}

	var allReviews []GithubReview
	for {
		err := client.Query(context.Background(), &query, variables)
		if err != nil {
			return GithubPRReviewSummary{
				query.Repository.PullRequest.ReviewDecision,
				allReviews,
			}, fmt.Errorf("getting reviewDecision: %f", err)
		}

		allReviews = append(allReviews, query.Repository.PullRequest.Reviews.Nodes...)
		// if we don't have a NextPage pointer, we have requested all pages
		if !query.Repository.PullRequest.Reviews.PageInfo.HasNextPage {
			break
		}
		// set the end cursor, so the next batch of reviews is going to be requested and not the same again
		variables["reviewCursor"] = githubv4.NewString(query.Repository.PullRequest.Reviews.PageInfo.EndCursor)
	}
	return GithubPRReviewSummary{
		query.Repository.PullRequest.ReviewDecision,
		allReviews,
	}, nil
}

func discardReviews(client *githubv4.Client, repo Repo, pull PullRequest) error {
	reviewStatus, err := getPRReviews(client, repo, pull)
	if err != nil {
		return err
	}

	// https://docs.github.com/en/graphql/reference/input-objects#dismisspullrequestreviewinput
	var mutation struct {
		DismissPullRequestReview struct {
			PullRequestReview struct {
				ID githubv4.ID
			}
		} `graphql:"dismissPullRequestReview(input: $input)"`
	}

	// dismiss every review one by one.
	// currently there is no way to dismiss them in one mutation.
	for _, review := range reviewStatus.Reviews {
		input := githubv4.DismissPullRequestReviewInput{
			PullRequestReviewID: review.ID,
			Message:             "DIMISSED FOR TESTING",
			ClientMutationID:    githubv4.NewString("grounded042"),
		}
		s, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("could not marshal json: %f", err)
		}
		fmt.Println("DISMISS", string(s))

		mutationResult := &mutation
		err = client.Mutate(context.Background(), mutationResult, input, nil)
		if err != nil {
			return fmt.Errorf("dismissing reviewDecision: %f", err)
		}

		m, err := json.Marshal(mutationResult)
		if err != nil {
			return fmt.Errorf("could not marshal json: %f", err)
		}
		fmt.Println("DISMISSED", string(m))
	}
	return nil
}
