dnl undivert transfers into the current diversion, including self and discard.
divert(`1')ONE divert(`2')TWO undivert(`1') END divert(`0')
divert(`3')SELF undivert(`3') END divert(`0')
divert(`4')DISCARD divert(`-1')undivert(`4')divert(`0')kept
