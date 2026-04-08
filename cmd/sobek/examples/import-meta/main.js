// import-meta — shows import.meta.url for both the entry and an imported module.
// The host (cmd/sobek) provides the URL via WithGetImportMetaProperties.
// Run: go run ./cmd/sobek examples/import-meta/main.js
import { libURL, greet } from "./lib.js";

console.log("entry  url:", import.meta.url);
console.log("lib    url:", libURL);
console.log(greet("World"));
