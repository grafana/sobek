// dynamic-import — loads modules at runtime based on a variable.
// Dynamic import() returns a Promise; the event loop in cmd/sobek resolves it.
// Run: go run ./cmd/sobek examples/dynamic-import/main.js
const plugins = ["shout", "whisper"];

for (const name of plugins) {
    // The specifier is constructed at runtime — only possible with dynamic import.
    const { transform } = await import(`./plugins/${name}.js`);
    console.log(`[${name}]`, transform("Hello from dynamic import"));
}
