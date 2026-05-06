CXX     = g++
CXXFLAGS = -O2 -std=c++17 -Wall

all: price_generator

price_generator: price_generator.cpp
	$(CXX) $(CXXFLAGS) -o $@ $<

clean:
	rm -f price_generator
