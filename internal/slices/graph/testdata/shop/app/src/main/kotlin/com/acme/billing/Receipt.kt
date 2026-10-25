package com.acme.billing

class Receipt(val invoice: Invoice) {
    fun print() = invoice.total()
}
