#include <iostream>
#include <random>
#include <vector>
#include <cmath>
#include <iomanip>

class RandomWalkPriceGenerator {
public:
    RandomWalkPriceGenerator(double initialPrice, double mu, double sigma, unsigned seed = std::random_device{}())
        : price(initialPrice), mu(mu), sigma(sigma), rng(seed), dist(0.0, 1.0) {}

    // Geometric Brownian Motion: dS = S * exp((mu - 0.5*sigma^2)*dt + sigma*sqrt(dt)*Z)
    double next(double dt = 1.0 / 252.0) {
        double z = dist(rng);
        double drift = (mu - 0.5 * sigma * sigma) * dt;
        double diffusion = sigma * std::sqrt(dt) * z;
        price *= std::exp(drift + diffusion);
        return price;
    }

    std::vector<double> generate(int steps, double dt = 1.0 / 252.0) {
        std::vector<double> prices;
        prices.reserve(steps);
        for (int i = 0; i < steps; ++i)
            prices.push_back(next(dt));
        return prices;
    }

    double currentPrice() const { return price; }
    void reset(double initialPrice) { price = initialPrice; }

private:
    double price;
    double mu;
    double sigma;
    std::mt19937 rng;
    std::normal_distribution<double> dist;
};

int main() {
    const double initialPrice = 100.0;
    const double mu    = 0.05;   // 5% annual drift
    const double sigma = 0.20;   // 20% annual volatility
    const int    steps = 252;    // one trading year

    RandomWalkPriceGenerator gen(initialPrice, mu, sigma);
    auto prices = gen.generate(steps);

    std::cout << std::fixed << std::setprecision(2);
    std::cout << "Day   0: " << initialPrice << "\n";
    for (int i = 0; i < steps; ++i)
        std::cout << "Day " << std::setw(3) << (i + 1) << ": " << prices[i] << "\n";

    return 0;
}
