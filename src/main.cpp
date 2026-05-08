#include <cassert>
#include <chrono>
#include <cstdint>
#include <iostream>
#include "OrderBook.h"
#include "PriceEngine.h"

int main() {
    // --- order book tests ---
    {
        OrderBook book;
        book.addLimitOrder(Side::SELL, 101.0, 5.0);
        book.addLimitOrder(Side::SELL, 102.0, 5.0);
        book.addLimitOrder(Side::BUY,  99.0, 5.0);

        assert(book.bestAsk() == 101.0);
        assert(book.bestBid() == 99.0);
        assert(book.spread()  == 2.0);

        // partial fill — consumes 3 of 5 at 101.0
        auto trades = book.addMarketOrder(Side::BUY, 3.0);
        assert(trades.size() == 1);
        assert(trades[0].price    == 101.0);
        assert(trades[0].quantity == 3.0);
        assert(book.bestAsk()     == 101.0);  // level still exists (2.0 remaining)

        // fill rest of 101 level + partial 102 level
        auto trades2 = book.addMarketOrder(Side::BUY, 4.0);
        assert(trades2.size()      == 2);
        assert(trades2[0].price    == 101.0);
        assert(trades2[0].quantity == 2.0);
        assert(trades2[1].price    == 102.0);
        assert(trades2[1].quantity == 2.0);
        assert(book.bestAsk()      == 102.0);  // 101 level fully consumed
        
        book.printBook();
    }
    // --- end tests ---

    PriceEngine engine(100.0, 42);

    auto baseTime = std::chrono::system_clock::now();

    for (int tick = 1; tick <= 1000; ++tick) {
        double price = engine.nextPrice();
        auto ts = std::chrono::duration_cast<std::chrono::seconds>(
            baseTime.time_since_epoch() + std::chrono::seconds(tick)
        ).count();

        std::cout << "{\"tick\":" << tick
                  << ",\"price\":" << price
                  << ",\"ts\":"    << ts
                  << "}\n";
    }

    return 0;
}
