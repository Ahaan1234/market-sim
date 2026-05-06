#pragma once

#include <random>

class PriceEngine {
public:
    PriceEngine(double startPrice, unsigned int seed);

    double nextPrice();
    double currentPrice() const;

private:
    double price_;
    std::mt19937 rng_;
    std::uniform_real_distribution<double> dist_;  // [-0.01, 0.01]

    static constexpr double kFloor = 10.0;
};
