// classes — ES6 classes with private fields imported from another module.
// Run: go run ./cmd/sobek examples/classes/main.js
import { Counter } from "./counter.js";

const c = new Counter(10);
c.increment(5).increment(3).decrement(2);
console.log(c.toString());      // Counter(16)
console.log("value:", c.value); // value: 16

c.reset();
console.log("after reset:", c.value); // after reset: 0
