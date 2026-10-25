package com.acme.billing

import com.acme.util.*
import com.acme.util.Money.Companion
import kotlin.math.max

class Invoice { fun total() = Money(max(1, 2)) }
