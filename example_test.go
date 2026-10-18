package carragent_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	carragent "carragent.com/go"
)

// These examples show the calls. They have no expected output, because they talk to the live API: they are compiled,
// not run.

func Example() {
	ctx := context.Background()
	client := carragent.NewClient()

	// Which agent follows up late invoices?
	results, err := client.Search(ctx, "late invoices", "")
	if err != nil {
		log.Fatal(err)
	}
	for _, hit := range results.Data {
		fmt.Println(hit.Kind, hit.ID, hit.Name)
	}

	// What does it do, and what does it leave to the owner?
	agent, err := client.GetAgent(ctx, "collections", carragent.English)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(agent.Summary)
	fmt.Println("Waits for the owner:", agent.WaitsForOwner)
	fmt.Println("Never:", agent.Never)

	// What does it cost? Prices are whole US cents, before taxes.
	plans, err := client.ListPlans(ctx, "")
	if err != nil {
		log.Fatal(err)
	}
	for _, plan := range plans.Data {
		if plan.SelfServe {
			fmt.Printf("%s: $%.2f a month\n", plan.Name, float64(plan.MonthlyCents)/100)
		}
	}
	fmt.Println("Free trial:", plans.Terms.TrialDays, "days")
}

func ExampleClient_IterAgents() {
	ctx := context.Background()
	client := carragent.NewClient()

	it := client.IterAgents(&carragent.ListAgentsParams{Lang: carragent.Spanish})
	for it.Next(ctx) {
		fmt.Println(it.Agent().ID, "-", it.Agent().Summary)
	}
	if err := it.Err(); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_Batch() {
	ctx := context.Background()
	client := carragent.NewClient()

	answers, err := client.Batch(ctx, []carragent.BatchItem{
		{ID: "plans", Path: "/plans"},
		{ID: "one", Path: "/agents/collections"},
		{Path: "/search?q=appointment%20reminders"},
	})
	if err != nil {
		log.Fatal(err) // the whole request was refused
	}
	for _, answer := range answers.Responses {
		fmt.Println(answer.ID, answer.Status)
	}

	// An item that failed carries its problem; Decode returns it as an error.
	var agent carragent.Agent
	if err := answers.Responses[1].Decode(&agent); err != nil {
		log.Fatal(err)
	}
	fmt.Println(agent.Name)
}

func ExampleClient_CreateJob() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := carragent.NewClient()

	accepted, err := client.CreateJob(ctx, carragent.OperationSearchCatalog, map[string]any{"q": "late invoices"})
	if err != nil {
		fmt.Println(err)
		return
	}
	job, err := client.WaitJob(ctx, accepted.ID, time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	var results carragent.SearchResults
	if err := job.Result.Decode(&results); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(results.Data), "matches")
}

func ExampleProblem() {
	client := carragent.NewClient()

	_, err := client.GetAgent(context.Background(), "no-such-agent", "")
	var problem *carragent.Problem
	if errors.As(err, &problem) {
		switch {
		case problem.RateLimited():
			time.Sleep(problem.RetryAfter) // the client never retries by itself
		case problem.Code == "agent_not_found":
			fmt.Println("no such agent:", problem.Detail)
		default:
			fmt.Println(problem.Status, problem.Code, problem.Title)
		}
	}
}

func ExampleClient_UseAnonymousIdentity() {
	ctx := context.Background()
	client := carragent.NewClient()

	// Registers this agent anonymously, gets a token for it and uses it from now on. The token lasts an hour. Keep the
	// registration (good for 24 hours) to get a new token without registering again.
	registration, err := client.RegisterAnonymous(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := client.UseAnonymousIdentity(ctx, registration); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_UseClientCredentials() {
	ctx := context.Background()
	client := carragent.NewClient()

	// An OAuth client from POST /api/oauth2/register (https://carragent.com/auth.md).
	_, err := client.UseClientCredentials(ctx, "client id", "client secret", carragent.WithScope("agents:read plans:read"))
	if err != nil {
		log.Fatal(err)
	}
}
