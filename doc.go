// Package carragent is a client for Carragent's public, read-only API: its AI agents for small businesses, and its
// plans and prices (https://carragent.com/developers).
//
// The API needs no account and no key. This package reads it. It does not sign anyone up, start a trial, change
// anything or reach a business's data. It uses only the standard library.
//
// The module path is carragent.com/go and the package is named carragent:
//
//	import carragent "carragent.com/go"
//
// # Reading
//
// Make a Client and call its methods. Every one takes a context first, and returns typed values:
//
//	client := carragent.NewClient()
//
//	results, err := client.Search(ctx, "late invoices", "")
//	if err != nil {
//		return err
//	}
//	for _, hit := range results.Data {
//		fmt.Println(hit.Kind, hit.ID, hit.Name)
//	}
//
//	agent, err := client.GetAgent(ctx, "collections", carragent.English)
//	// agent.Summary, agent.WaitsForOwner, agent.Never
//
//	plans, err := client.ListPlans(ctx, "")
//	// Prices are whole US cents, before taxes: plan.MonthlyCents, plan.YearlyCents.
//
// Text comes in English, Spanish or French (Lang); the empty Lang is English. Client.IterAgents walks every agent,
// Client.Batch runs up to 20 reads in one request, and Client.CreateJob, Client.GetJob and Client.WaitJob are the
// REST pattern for slow operations, which these fast reads follow so that code written for one works unchanged.
//
// # Errors
//
// An error answer from the API is a *Problem (an RFC 9457 problem document). Branch on its Code, which is stable:
//
//	var problem *carragent.Problem
//	if errors.As(err, &problem) {
//		if problem.RateLimited() {
//			time.Sleep(problem.RetryAfter)
//		}
//	}
//
// The client never retries by itself. A request that gets no answer returns the transport's error, which wraps the
// standard ones: errors.Is(err, context.DeadlineExceeded) is true when the context ran out.
//
// # Tokens
//
// A token is optional. It raises the rate limit from 60 requests a minute per IP address to 300 a minute per client,
// and reads the same public catalog. Client.UseAnonymousIdentity registers the agent anonymously and gets one;
// Client.UseClientCredentials does it for an OAuth client. Both set Client.Token. A token lasts an hour.
//
// # Concurrency
//
// A Client is safe for concurrent use as long as its fields are not changed meanwhile. UseAnonymousIdentity and
// UseClientCredentials change Token, so call them before sharing the client between goroutines.
package carragent
