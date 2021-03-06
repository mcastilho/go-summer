// Hidden Markov Model toolkit
package hmm

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"dbconn"
	"redis/redis"
)

const (
	// keys' markers for Redis DB
	DBA = "@@#"  // A
	DBB = "@#@"  // B
	DBPI = "@@@" // Pi
	FS = "$"     // Feature separator
)


type HiddenMM struct {
	// number of states
	N int
	// number of symbols for feature 0
	M0 int
	// number of symbols for feature 1
	M1 int
	// transition probabilities N x N
	A [][]float64
	// emission probabilities N x M0 x M1
	B [][][]float64
	// initial probability distribution vector
	Pi []float64
}


func InitHMM(N, M0, M1 int) HiddenMM {
	fmt.Println("Creating model")
	Pi := make([]float64, N, N)

	A := make([][]float64, N, N)
	for i := range A {
		A[i] = make([]float64, N, N)

		for j := range A[i] {
			A[i][j] = 1.0 / float64(N)
		}
	}

	B := make([][][]float64, N, N)
	for i := range B {
		B[i] = make([][]float64, M0, M0)

		for j := range B[i] {
			B[i][j] = make([]float64, M1, M1)

			for k := range B[i][j] {
				B[i][j][k] = 1.0 / float64(M0 * M1)
			}
		}
	}
	return HiddenMM{N, M0, M1, A, B, Pi}
}


func CheckConvergence(old_likelihood float64, new_likelihood float64, current_iter int, max_iter int, tolerance float64) bool {
	if tolerance > 0 {
		if math.Abs(old_likelihood - new_likelihood) <= tolerance {
			return true
		}

		if max_iter > 0 {
			if current_iter >= max_iter { return true }
		}

	} else {
		if current_iter == max_iter { return true }
	}

	if math.IsNaN(new_likelihood) || math.IsInf(new_likelihood, 0) {
		return true
	}

	return false
}


// Kalman filter
// Calculate probability of generated sequence
// http://en.wikipedia.org/wiki/Forward_algorithm
func (h *HiddenMM) Forward(observation [][]int, c []float64) [][]float64 {
	fwd := make([][]float64, len(observation))

	for i := range fwd {
		fwd[i] = make([]float64, h.N)
	}

	// STEP 1
	// init
	for i := 0; i < h.N; i++ {
		if observation[0][0] * observation[0][1] != 0 {
			fwd[0][i] = h.Pi[i] * h.B[i][observation[0][0]][observation[0][1]]
		} else {
			fwd[0][i] = h.Pi[i]
		}
		c[0] += fwd[0][i]
	}

	// scaling
	if c[0] != 0 {
		for i := 0; i < h.N; i++ {
			fwd[0][i] /= c[0]
		}
	}

	// STEP 2
	// induction
	for t := 1; t < len(observation); t++ {
		for i := 0; i < h.N; i++ {
			for j := 0; j < h.N; j++ {

				p := 0.0
				// check if state is silent
				if observation[t][0] * observation[t][1] != 0 {
					p = h.A[j][i] * h.B[i][observation[t][0]][observation[t][1]]
				} else {
					p = h.A[j][i]
				}
				fwd[t][i] += fwd[t - 1][j] * p
			}
			c[t] += fwd[t][i]  // likelihood
		}
		// scaling
		if c[t] != 0 {
			for i := 0; i < h.N; i++ {
				fwd[t][i] /= c[t]
			}
		}
	}
	return fwd
}


// Kalman smoothing
// Backward variables - use the same 'forward' scaling factor
func (h *HiddenMM) Backward(observation [][]int, c []float64) [][]float64 {
	T := len(observation)
	bwd := make([][]float64, T)

	for i := range bwd {
		bwd[i] = make([]float64, h.N)
	}
	// STEP 1
	// init
	for i := 0; i < h.N; i++ {
		bwd[T - 1][i] = 1.0 / c[T - 1]
	}

	// STEP 2
	// induction
	for t := T - 2; t >= 0; t-- {
		for i := 0; i < h.N; i++ {
			sum := 0.0
			for j := 0; j < h.N; j++ {
				
				p := 0.0
				// check if state is silent
				if observation[t + 1][0] * observation[t + 1][1] != 0 {
					p = h.B[j][observation[t + 1][0]][observation[t + 1][1]]
				} else {
					p = 1.0
				}
				sum += h.A[i][j] * p * bwd[t + 1][j]
			}
			bwd[t][i] += sum / c[t]
		}
	}
	return bwd
}


// main algorithm for learning HMM parameters from given observations
// Supervised learning - we know states -> counting strategy
func (h *HiddenMM) Learn(observations [][][]int) {
	// for each sequence of observations
	scale := make([]int, h.N, h.N)

	for n := range observations {
		// time - t == i (number of state)
		for t := range observations[n] {
			sposition := observations[n][t][0]
			slength := observations[n][t][1]

			scale[t]++

			if sposition * slength != 0 {
				h.Pi[t] += 1.0

				for i := 0; i < slength; i++ {
					h.B[t][sposition][i] += 1.0
				}

				for i := 0; i < sposition - 1; i++ {
					h.B[t][i][slength] += 1.0
				}
			}

			for j := t + 1; j < len(observations[n]); j++ {
				nextposition := observations[n][j][0]
				nextlength := observations[n][j][1]

				if nextposition * nextlength != 0 {
					h.A[t][j] += 1.0
					break
				}
			}
		}
	}

	for i := 0; i < h.N; i++ {
		if scale[i] == 0 { continue }

		h.Pi[i] /= float64(scale[i])

		for j := 0; j < h.N; j++ {
			h.A[i][j] /= float64(scale[i])
		}

		for j := 0; j < h.M0; j++ {
			for k := 0; k < h.M1; k++ {
				h.B[i][j][k] /= float64(scale[i])
			}
		}
	}
}


// Unsupervised Learning - update created model
func (h *HiddenMM) UpdateModel(observations [][][]int, iterations int, tolerance float64) float64 {
	
	if tolerance + float64(iterations) == 0.0 {
		return 0.0
	}

	iter := 1
	stop := false

	// init
	epsilon := make([][][][]float64, len(observations))
	gamma := make([][][]float64, len(observations))

	for i := range observations {
		epsilon[i] = make([][][]float64, len(observations[i]))
		gamma[i] = make([][]float64, len(observations[i]))

		for j := range observations[i] {
			epsilon[i][j] = make([][]float64, h.N)
			gamma[i][j] = make([]float64, h.N)

			for k := 0; k < h.N; k++ {
				epsilon[i][j][k] = make([]float64, h.N)
			}
		}
	}
	// initial log-likelihood
	old_likelihood := math.SmallestNonzeroFloat64
	new_likelihood := float64(0)

	// maintain loop until reaching tolerance
	// or number of iterations
	for !stop {
		// for each sequence of observations
		for i := range observations {
			scaling := make([]float64, len(observations[i]))

			// I STEP
			// calculate forward & backward probability
			fwd := h.Forward(observations[i], scaling)
			bwd := h.Backward(observations[i], scaling)

			// II STEP
			// transmission & emission pairs

			// gamma
			for j := range observations[i] {
				s := float64(0)

				for k := 0; k < h.N; k++ {
					gamma[i][j][k] = fwd[j][k] * bwd[j][k]
					s += gamma[i][j][k]
				}
				
				// scaling
				if s != 0 {
					for k := 0; k < h.N; k++ {
						gamma[i][j][k] = gamma[i][j][k] / s
					}
				}
			}

			// epsilon
			for j := 0; j < len(observations[i]) - 1; j++ {
				s := 0.0

				for k := 0; k < h.N; k++ {
					for l := 0; l < h.N; l++ {
						epsilon[i][j][k][l] = fwd[j][k] * h.A[k][l] * bwd[j + 1][l] * h.B[l][observations[i][j + 1][0]][observations[i][j + 1][1]]
						s += epsilon[i][j][k][l]
					}
				}
				
				if s != 0 {
					for k := 0; k < h.N; k++ {
						for l := 0; l < h.N; l++ {
							epsilon[i][j][k][l] = epsilon[i][j][k][l] / s
						}
					}
				}
			}
			// log-likelihood for the sequence
			for t := range scaling {
				new_likelihood += math.Log(scaling[t])
			}
		}

		// average likelihood
		new_likelihood /= float64(len(observations))

		// check convergence
		if CheckConvergence(old_likelihood, new_likelihood, iter, iterations, tolerance) {
			stop = true
		} else {
			// STEP 3
			// parameter re-estimation
			iter++
			old_likelihood = new_likelihood
			new_likelihood = 0.0

			// init probabilities
			for k := 0; k < h.N; k++ {
				sum := 0.0
				for i := range observations {
					sum += gamma[i][0][k]
				}
				h.Pi[k] = sum / float64(len(observations))
			}
			
			// transition probabilities
			for i := 0; i < h.N; i++ {
				for j := 0; j < h.N; j++ {
					den, num := 0.0, 0.0
					for k := range observations {
						T := len(observations[k])

						for l := 0; l < T - 1; l++ {
							num += epsilon[k][l][i][j]
						}
						
						for l := 0; l < T - 1; l++ {
							den += gamma[k][l][i]
						}
					}
					
					if den == 0.0 {
						h.A[i][j] = 0.0
					} else {
						h.A[i][j] = num / den
					}
				}
			}

			// emission probabilities
			for i := 0; i < h.N; i++ {
				for j := 0; j < h.M; j++ {
					
					den, num := 0.0, 0.0
					
					for k := range observations {
						
						for l := range observations[k] {
							if observations[k][l] == j {
								num += gamma[k][l][i]
							}
						}
						
						for l := range observations[k] {
							den += gamma[k][l][i]
						}
					}
					
					if num == 0.0 {
						h.B[i][j][0], h.B[i][j][1] = 1e-10, 1e-10 
					} else {
						h.B[i][j][0], h.B[i][j][1] = num / den, num / den
					}
				}
			}
		}
	}
	// return the model avg log-likelihood
	return new_likelihood
}
//


// Decode hidden states
func (h *HiddenMM) Viterbi(observation [][]int, probability *float64) []int {
	
	T := len(observation)
	min_state := 0
	min_weight := 0.0
	weight := 0.0

	s := make([][]int, h.N)
	for i := range s {
		s[i] = make([]int, T)
	}	

	a := make([][]float64, h.N)
	for i := range a {
		a[i] = make([]float64, T)
	}
	
	//Init
	for i := 0; i < h.N; i++ {
		a[i][0] = (-1.0 * math.Log(h.Pi[i])) - math.Log(h.B[i][observation[0][0]][observation[0][1]])
	}

	//Induction
	for t := 1; t < T; t++ {
		for j := 0; j < h.N; j++ {
			min_state = 0
			min_weight = a[0][t - 1] - math.Log(h.A[0][j])

			for i := 0; i < h.N; i++ {
				weight = a[i][t - 1] - math.Log(h.A[i][j])

				if weight < min_weight {
					min_state, min_weight = i, weight
				}
			}
			spositon, slength := observation[t][0], observation[t][1]

			a[j][t] = min_weight - math.Log(h.B[j][spositon][slength])
			s[j][t] = min_state
		}
	}

	// Min for the last element of observation
	min_state = 0
	min_weight = a[0][T - 1]

	for i := 1; i < h.N; i++ {
		if a[i][T - 1] < min_weight {
			min_state = i
			min_weight = a[i][T - 1]
		}
	}
	//Traceback
	path := make([]int, T)
	path[T - 1] = min_state

	for t := T - 2; t >= 0; t-- {
		path[t] = s[path[t + 1]][t + 1]
	}

	*probability = math.Exp(-min_weight)
	fmt.Println("Minimal probability", *probability)
	return path
}


func (h *HiddenMM) Evaluate(observation [][]int, logarithm bool) float64 {
    // Forward algorithm
    likelihood := float64(0);
    coefficients := make([]float64, len(observation))

    // Compute forward probabilities
    h.Forward(observation, coefficients)

    for _, v := range coefficients {
    	likelihood += math.Log(v);
    }
    // Return the sequence probability
    if logarithm {
    	return likelihood
    }
    return math.Exp(likelihood)
}


func Int2Str(i int) string {
	return strconv.FormatInt(int64(i), 10)
}


func ManageStoring(c, quit chan int, rows int) {

	for i := 0; i < rows; i++ {
		c <- i
	}
	close(c)

	<-quit
	return
}


func (h *HiddenMM) StoreRow(i int, connection redis.Conn) {

	connection.Do("RPUSH", DBPI, h.Pi[i])	
	connection.Send("MULTI")

	for j := range h.A[i] {
		connection.Send("RPUSH", DBA + Int2Str(i), h.A[i][j])
	}

	for j := range h.B[i] {
		for k := range h.B[i][j] {
			connection.Send("RPUSH", DBB + Int2Str(i) + FS + Int2Str(j), h.B[i][j][k])
		}
	}
	connection.Do("EXEC")
}


func (h *HiddenMM) Store() {
	pool := dbconn.Pool
	connection := pool.Get()
	
	c, quit := make(chan int), make(chan int)

	go func() {
		for i := range c {			
			h.StoreRow(i, connection)
		}
		quit <- 0
	}()
	ManageStoring(c, quit, h.N)

	connection.Close()
}


func Load(N, M0, M1 int) HiddenMM {
	pool := dbconn.Pool
	connection := pool.Get()

	A := make([][]float64, N, N)

	for i := range A {
		loadedA, err := redis.Values(connection.Do("LRANGE", DBA + Int2Str(i), 0, -1))

		if err != nil {
			fmt.Println("Error while reading A[", i, "]")
			os.Exit(1)
		}
		A[i] = make([]float64, N, N)
		
		for j := range A[i] {
			A[i][j], _ = redis.Float64(loadedA[j], err)
		}
	}

	B := make([][][]float64, N, N)

	for i := range B {
		B[i] = make([][]float64, M0, M0)
		
		for j := range B[i] {
			loadedB, err := redis.Values(connection.Do("LRANGE", DBB + Int2Str(i) + FS + Int2Str(j), 0, -1))

			if err != nil {
				fmt.Println("Error while reading B[", i, "][", j,"]")
				os.Exit(1)
			}

			B[i][j] = make([]float64, M1, M1)
			for k := range B[i][j] {
				B[i][j][k], _ = redis.Float64(loadedB[j], err)	
			}
		}
	}

	loadedPi, err := redis.Values(connection.Do("LRANGE", DBPI, 0, -1))

	if err != nil {
		fmt.Println("Error while reading Pi")
		os.Exit(1)
	}
	Pi := make([]float64, N, N)

	for i := range Pi {
		Pi[i], _ = redis.Float64(loadedPi[i], err)
	}

	connection.Close()

	return HiddenMM{N, M0, M1, A, B, Pi}
}
