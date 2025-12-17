import unittest

import calc


class TestCalc(unittest.TestCase):
    def test_add(self) -> None:
        self.assertEqual(calc.add(2, 2), 4)

    def test_mul(self) -> None:
        self.assertEqual(calc.mul(3, 4), 12)


if __name__ == "__main__":
    unittest.main()

