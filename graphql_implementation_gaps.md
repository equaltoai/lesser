# GraphQL API Implementation Gaps

This document outlines the implementation gaps discovered during a review of unused code warnings in the GraphQL API layer. These findings highlight areas where features are either incomplete, buggy, or contain dead code that needs to be addressed.

## 1. Unimplemented GraphQL Resolvers

These are features that are defined in the GraphQL schema but are not yet fully implemented in the resolver code.

### 1.1. `Hashtag.relatedHashtags`

*   **Issue**: The `getRelatedHashtags` function in `graph/helpers.go` is unused, despite the GraphQL schema defining a `relatedHashtags` field on the `Hashtag` type.
*   **Analysis**: This is a planned feature that has a backend implementation but is not yet connected to the GraphQL API. The resolver for this field is missing.
*   **Required Action**: Implement the GraphQL resolver for `Hashtag.relatedHashtags` in `graph/schema.resolvers.go`. This resolver should call the existing `getRelatedHashtags` function to fetch the data.

### 1.2. `aiAnalysis` Query

*   **Issue**: The helper functions `convertToTextAnalysis`, `convertToImageAnalysis`, `convertToAIDetection`, and `convertToSpamAnalysis` in `graph/schema.resolvers.go` are all unused.
*   **Analysis**: These functions are intended to be used within the resolver for the `aiAnalysis` query, which is currently a stub. The GraphQL schema defines this query and its corresponding `AIAnalysis` return type.
*   **Required Action**: Implement the `aiAnalysis` query resolver in `graph/schema.resolvers.go`. This resolver should call the AI service to get analysis data and then use the `convertTo...` helper functions to map the results into the `AIAnalysis` GraphQL model.

## 2. Bugs and Refactoring Opportunities

This section details code that is buggy or requires refactoring to function correctly.

### 2.1. `CreateModerationPattern` Mutation

*   **Issue**: In the `CreateModerationPattern` mutation resolver in `graph/mutation_resolvers_moderation.go`, several fields of a `models.ModerationPattern` struct are written to, but the struct itself is never used.
*   **Analysis**: This is a bug that causes the writes to be ineffective. A local variable is created and populated, but a different variable is used for the database operation.
*   **Required Action**: Refactor the `CreateModerationPattern` function to consolidate the logic into the `storage.ModerationPattern` object that is actually saved to the database. The unused `models.ModerationPattern` variable should be removed.

## 3. Dead Code and Unused Logic

This section identifies code that is not currently used and may need to be removed or have its corresponding logic implemented.

### 3.1. `ml-training-processor` Constants

*   **Issue**: The constants `eventNameRemove` and `statusSubmitted` in `cmd/ml-training-processor/main.go` are unused.
*   **Analysis**: The `ml-training-processor` lambda does not have logic to handle "REMOVE" events from DynamoDB streams or "SUBMITTED" job statuses.
*   **Required Action**: A decision needs to be made on whether the logic to handle these cases is missing and should be implemented, or if these constants are unnecessary for this processor's scope and should be removed to clean up the code.

This document should serve as a clear guide for the engineering team to prioritize and plan the implementation of these missing pieces of the GraphQL API.
