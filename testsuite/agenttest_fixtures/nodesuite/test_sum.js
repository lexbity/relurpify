const assert = require("assert");
const { add, mul } = require("./sum");

assert.strictEqual(add(2, 2), 4);
assert.strictEqual(mul(3, 4), 12);

console.log("ok");

