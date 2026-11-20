package portal

// Known is the list of real university flows the server registers at start.
//
// It is empty on purpose: a flow must be written from the live portal, page
// by page, with a test account, and verified with a dry run (RunOptions{DryRun:
// true}) before it may submit for students. To add one:
//
//  1. Create a test account on the portal and walk through an application.
//  2. Write its Flow (see fakeFlow in runner_test.go for a complete example:
//     register, email code, forms, uploads, review, submit, confirmation).
//  3. Run it in dry-run mode against the live portal and check the screenshot.
//  4. Add it here; universities with the same portal id start using it.
var Known []Flow
