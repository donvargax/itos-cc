from board import low


def test_low():
    assert low(3)
    assert not low(4)
