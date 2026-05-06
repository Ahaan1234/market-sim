#include <chrono>
#include <cstdint>
#include <iostream>
#include "PriceEngine.h"

int main() {
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
