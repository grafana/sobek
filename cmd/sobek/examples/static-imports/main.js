// static-imports — shows named imports and re-exports across modules.
// Run: go run ./cmd/sobek examples/static-imports/main.js
import { add, subtract, multiply, PI, circleArea } from "./math.js";

console.log("3 + 4 =", add(3, 4));
console.log("10 - 3 =", subtract(10, 3));
console.log("6 × 7 =", multiply(6, 7));
console.log("PI ≈", PI);
console.log("Area of circle r=5:", circleArea(5).toFixed(4));
