package smoke

import kotlin.test.Test
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class BoardTest {
    @Test
    fun lowIsThree() {
        assertTrue(low(3))
        assertFalse(low(4))
    }
}
